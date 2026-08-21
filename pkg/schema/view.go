package schema

import (
	"bytes"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/pseudomuto/housekeeper/pkg/compare"
	"github.com/pseudomuto/housekeeper/pkg/format"
	"github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/pseudomuto/housekeeper/pkg/utils"
)

const (
	// ViewDiffCreate indicates a view needs to be created
	ViewDiffCreate ViewDiffType = "CREATE"
	// ViewDiffDrop indicates a view needs to be dropped
	ViewDiffDrop ViewDiffType = "DROP"
	// ViewDiffAlter indicates a view needs to be altered (only for materialized views using ALTER TABLE MODIFY QUERY)
	ViewDiffAlter ViewDiffType = "ALTER"
	// ViewDiffRename indicates a view needs to be renamed (uses RENAME TABLE for both regular and materialized views)
	ViewDiffRename ViewDiffType = "RENAME"
)

type (
	// ViewDiff represents a difference between current and target view states.
	// It handles both regular views and materialized views, with special handling
	// for materialized views which can only be altered using ALTER TABLE MODIFY QUERY.
	ViewDiff struct {
		DiffBase                 // Embeds Type, Name, NewName, Description, UpSQL, DownSQL
		Current        *ViewInfo // Current state (nil if view doesn't exist)
		Target         *ViewInfo // Target state (nil if view should be dropped)
		IsMaterialized bool      // True if this is a materialized view
	}

	// ViewDiffType represents the type of view difference
	ViewDiffType string

	// ViewInfo represents parsed view information extracted from DDL statements.
	// This structure contains all the properties needed for view comparison and
	// migration generation, including whether it's a materialized view.
	ViewInfo struct {
		Name           string                 // View name
		Database       string                 // Database name (empty for default database)
		Cluster        string                 // Cluster name if specified (empty if not clustered)
		IsMaterialized bool                   // True if this is a materialized view
		OrReplace      bool                   // True if created with OR REPLACE
		Query          string                 // Query string for validation compatibility
		Statement      *parser.CreateViewStmt // Full parsed CREATE VIEW statement for deep comparison
		// Functions is the lowercase-keyed SQL user-defined function registry
		// from the target schema, used to inline UDF calls in AsSelect before
		// comparison (see udf_inline.go for why this is required: ClickHouse
		// itself inlines UDF calls into a view's stored create_table_query,
		// so the un-inlined source DDL never structurally matches live state
		// without this expansion). nil for views where inlining wasn't set up
		// (comparison then falls back to comparing as-declared).
		Functions map[string]*FunctionInfo
	}
)

// GetName implements SchemaObject interface.
// Returns the full view name (database.name or just name).
func (v *ViewInfo) GetName() string {
	if v.Database != "" {
		return v.Database + "." + v.Name
	}
	return v.Name
}

// GetCluster implements SchemaObject interface.
func (v *ViewInfo) GetCluster() string {
	return v.Cluster
}

// PropertiesMatch implements SchemaObject interface.
// Returns true if the two views have identical properties (excluding name).
func (v *ViewInfo) PropertiesMatch(other SchemaObject) bool {
	otherView, ok := other.(*ViewInfo)
	if !ok {
		return false
	}
	return viewsHaveSameProperties(v, otherView)
}

// compareViews compares current and target schemas to find view differences.
// It analyzes CREATE VIEW statements from both schemas and generates appropriate
// migration operations including CREATE, DROP, ALTER (for materialized views), and RENAME.
//
// For materialized views that have changed:
// - Uses ALTER TABLE name MODIFY QUERY for content changes (not CREATE OR REPLACE)
// - Uses standard CREATE/DROP for creation/deletion
// - Uses RENAME TABLE for renames
//
// For regular views that have changed:
// - Uses CREATE OR REPLACE for content changes
// - Uses standard CREATE/DROP for creation/deletion
// - Uses RENAME TABLE for renames
func compareViews(current, target *parser.SQL) ([]*ViewDiff, error) { // nolint: unparam
	// Extract views from both schemas
	currentViews := extractViewsFromSQL(current)
	targetViews := extractViewsFromSQL(target)

	// Attach the target's function registry to every view on both sides so
	// comparison can inline UDF calls before diffing (see udf_inline.go).
	// Sourced from target only: current's AsSelect already reflects
	// ClickHouse's own inlined form and has no named UDF calls left to
	// resolve, so inlining it is a no-op regardless of which side's registry
	// is used.
	functions := lowercaseFunctionKeys(extractFunctionInfoAsMap(target))
	for _, v := range currentViews {
		v.Functions = functions
	}
	for _, v := range targetViews {
		v.Functions = functions
	}

	// Mirror the table-side cluster reconciliation: rewrite live-DB cluster names
	// to the source-side cluster (typically a server macro like '{cluster}'),
	// otherwise the macro vs resolved-name mismatch causes spurious DROP+CREATE on
	// every diff. See inferSchemaClusterFromStrings for the resolution policy.
	ReconcileClusters(maps.Values(currentViews), maps.Values(targetViews),
		func(v *ViewInfo) string { return v.Cluster },
		func(v *ViewInfo, c string) { v.Cluster = c },
	)

	var diffs []*ViewDiff

	// Find views to create, drop, alter, or rename using helper functions
	createD, err := findViewsToCreate(targetViews, currentViews)
	if err != nil {
		return nil, err
	}
	diffs = append(diffs, createD...)

	dropD, err := findViewsToDrop(currentViews, targetViews)
	if err != nil {
		return nil, err
	}
	diffs = append(diffs, dropD...)

	alterD, err := findViewsToAlterOrRename(currentViews, targetViews)
	if err != nil {
		return nil, err
	}
	diffs = append(diffs, alterD...)

	return diffs, nil
}

// findViewsToCreate finds views that need to be created (exist in target but not in current)
func findViewsToCreate(targetViews, currentViews map[string]*ViewInfo) ([]*ViewDiff, error) {
	// Count views to create for pre-allocation
	createCount := 0
	for name := range targetViews {
		if _, exists := currentViews[name]; !exists {
			createCount++
		}
	}

	diffs := make([]*ViewDiff, 0, createCount)

	// Sort view names for deterministic order
	viewNames := make([]string, 0, createCount)
	for name := range targetViews {
		if _, exists := currentViews[name]; !exists {
			viewNames = append(viewNames, name)
		}
	}
	sort.Strings(viewNames)

	for _, name := range viewNames {
		targetView := targetViews[name]
		// Validate create operation
		if err := validateViewOperation(nil, targetView); err != nil {
			return nil, err
		}

		diff := &ViewDiff{
			DiffBase: DiffBase{
				Type:        string(ViewDiffCreate),
				Name:        name,
				Description: fmt.Sprintf("Create %s %s", getViewType(targetView), name),
				UpSQL:       generateCreateViewSQL(targetView),
				DownSQL:     generateDropViewSQL(targetView),
			},
			Target:         targetView,
			IsMaterialized: targetView.IsMaterialized,
		}
		diffs = append(diffs, diff)
	}

	return diffs, nil
}

// findViewsToDrop finds views that need to be dropped (exist in current but not in target)
func findViewsToDrop(currentViews, targetViews map[string]*ViewInfo) ([]*ViewDiff, error) {
	// Count views to drop for pre-allocation
	dropCount := 0
	for name := range currentViews {
		if _, exists := targetViews[name]; !exists {
			dropCount++
		}
	}

	diffs := make([]*ViewDiff, 0, dropCount)

	// Sort view names for deterministic order
	viewNames := make([]string, 0, dropCount)
	for name := range currentViews {
		if _, exists := targetViews[name]; !exists {
			viewNames = append(viewNames, name)
		}
	}
	sort.Strings(viewNames)

	for _, name := range viewNames {
		currentView := currentViews[name]
		// Validate drop operation
		if err := validateViewOperation(currentView, nil); err != nil {
			return nil, err
		}

		diff := &ViewDiff{
			DiffBase: DiffBase{
				Type:        string(ViewDiffDrop),
				Name:        name,
				Description: fmt.Sprintf("Drop %s %s", getViewType(currentView), name),
				UpSQL:       generateDropViewSQL(currentView),
				DownSQL:     generateCreateViewSQL(currentView),
			},
			Current:        currentView,
			IsMaterialized: currentView.IsMaterialized,
		}
		diffs = append(diffs, diff)
	}

	return diffs, nil
}

// findViewsToAlterOrRename finds views that need to be altered or renamed
func findViewsToAlterOrRename(currentViews, targetViews map[string]*ViewInfo) ([]*ViewDiff, error) {
	diffs := make([]*ViewDiff, 0, len(currentViews))

	// Use generic rename detection algorithm
	renames, processedCurrent, processedTarget := DetectRenames(currentViews, targetViews)

	// Create rename diffs
	for _, rename := range renames {
		currentView := currentViews[rename.OldName]
		targetView := targetViews[rename.NewName]

		// Validate rename operation
		if err := validateViewOperation(currentView, targetView); err != nil {
			return nil, err
		}

		diff := &ViewDiff{
			DiffBase: DiffBase{
				Type:        string(ViewDiffRename),
				Name:        rename.OldName,
				NewName:     rename.NewName,
				Description: fmt.Sprintf("Rename %s %s to %s", getViewType(currentView), rename.OldName, rename.NewName),
				UpSQL:       generateRenameViewSQL(currentView, targetView),
				DownSQL:     generateRenameViewSQL(targetView, currentView),
			},
			Current:        currentView,
			Target:         targetView,
			IsMaterialized: currentView.IsMaterialized,
		}
		diffs = append(diffs, diff)
	}

	// Now check for alterations using processed maps (excludes renamed views)
	var currentNames []string
	for name := range processedCurrent {
		if _, exists := processedTarget[name]; exists {
			currentNames = append(currentNames, name)
		}
	}
	sort.Strings(currentNames)

	for _, name := range currentNames {
		currentView := processedCurrent[name]
		targetView := processedTarget[name]

		// Validate operation before proceeding
		if err := validateViewOperation(currentView, targetView); err != nil {
			return nil, err
		}

		if !viewsAreEqual(currentView, targetView) {
			// View needs to be altered
			var upSQL, downSQL string

			// For materialized views, use DROP+CREATE (more reliable than ALTER TABLE MODIFY QUERY)
			// For regular views, use CREATE OR REPLACE
			if currentView.IsMaterialized {
				upSQL = generateDropViewSQL(currentView) + "\n\n" + generateCreateViewSQL(targetView)
				downSQL = generateDropViewSQL(targetView) + "\n\n" + generateCreateViewSQL(currentView)
			} else {
				upSQL = generateCreateOrReplaceViewSQL(targetView)
				downSQL = generateCreateOrReplaceViewSQL(currentView)
			}

			diff := &ViewDiff{
				DiffBase: DiffBase{
					Type:        string(ViewDiffAlter),
					Name:        name,
					Description: fmt.Sprintf("Alter %s %s", getViewType(currentView), name),
					UpSQL:       upSQL,
					DownSQL:     downSQL,
				},
				Current:        currentView,
				Target:         targetView,
				IsMaterialized: currentView.IsMaterialized,
			}

			diffs = append(diffs, diff)
		}
	}

	return diffs, nil
}

// extractViewsFromSQL extracts all view information from parsed SQL
func extractViewsFromSQL(sql *parser.SQL) map[string]*ViewInfo {
	views := make(map[string]*ViewInfo)

	for _, stmt := range sql.Statements {
		if stmt.CreateView != nil {
			// Extract query string for validation - simplified approach
			queryStr := ""
			if stmt.CreateView.AsSelect != nil {
				// For now, we'll use a simple placeholder since we don't have String() method
				queryStr = "SELECT ..." // TODO: Implement proper query string extraction if needed
			}

			view := &ViewInfo{
				Name:           normalizeIdentifier(stmt.CreateView.Name),
				Database:       normalizeDefaultDatabase(getStringValue(stmt.CreateView.Database)),
				Cluster:        getStringValue(stmt.CreateView.OnCluster),
				IsMaterialized: stmt.CreateView.Materialized,
				OrReplace:      stmt.CreateView.OrReplace,
				Query:          queryStr, // For validation compatibility
				Statement:      stmt.CreateView,
			}

			// Create full name (database.name or just name)
			fullName := view.Name
			if view.Database != "" {
				fullName = view.Database + "." + view.Name
			}

			views[fullName] = view
		}
	}

	return views
}

// viewsAreEqual compares two ViewInfo structures for equality
func viewsAreEqual(current, target *ViewInfo) bool {
	if current.Name != target.Name ||
		current.Database != target.Database ||
		current.IsMaterialized != target.IsMaterialized {
		return false
	}

	// Note: OrReplace is ignored because it's a creation-time directive
	// that's not preserved in ClickHouse's stored object definitions

	if current.Cluster != target.Cluster {
		return false
	}

	// Compare the full statements for deep equality
	// This includes comparing the SELECT clause and all other properties
	result := viewStatementsAreEqual(current.Statement, target.Statement, target.Functions)

	// Debug output disabled for now
	// if !result && current.Statement != nil && target.Statement != nil {
	//     fmt.Printf("DEBUG: View %s.%s has differences\n", current.Database, current.Name)
	// }

	return result
}

// viewsHaveSameProperties compares views ignoring the name (used for rename detection)
func viewsHaveSameProperties(view1, view2 *ViewInfo) bool {
	if view1.Database != view2.Database ||
		view1.IsMaterialized != view2.IsMaterialized {
		return false
	}

	// Note: OrReplace is ignored because it's a creation-time directive
	// that's not preserved in ClickHouse's stored object definitions

	if view1.Cluster != view2.Cluster {
		return false
	}

	// Compare statements ignoring names. Both sides carry the same
	// target-derived registry (see compareViews), so either is fine to use.
	functions := view1.Functions
	if functions == nil {
		functions = view2.Functions
	}
	return viewStatementsHaveSameProperties(view1.Statement, view2.Statement, functions)
}

// viewStatementsAreEqual compares two CREATE VIEW statements for complete
// equality. `functions` (lowercase-keyed) is used to inline SQL
// user-defined function calls in stmt2's AsSelect before comparison — see
// udf_inline.go.
func viewStatementsAreEqual(stmt1, stmt2 *parser.CreateViewStmt, functions map[string]*FunctionInfo) bool {
	// Note: IfNotExists is ignored because it's a creation-time directive
	// that's not preserved in ClickHouse's stored object definitions

	// Compare REFRESH strategy (refreshable materialized views)
	if !refreshClausesAreEqual(stmt1.Refresh, stmt2.Refresh) {
		return false
	}

	// Compare TO clauses (materialized views only)
	if getViewTableTargetValue(stmt1.To) != getViewTableTargetValue(stmt2.To) {
		return false
	}

	// Compare ENGINE clauses with ClickHouse limitations in mind
	if !engineClausesAreEqualWithTolerance(stmt1.Engine, stmt2.Engine) {
		return false
	}

	// Compare POPULATE with ClickHouse limitations in mind
	// ClickHouse doesn't preserve POPULATE directive in stored definitions
	// Allow differences since ClickHouse doesn't preserve POPULATE in system.tables
	_ = stmt1.Populate // Acknowledge that we're intentionally ignoring this field
	_ = stmt2.Populate

	// EMPTY is a create-time directive for refreshable MVs; treat like POPULATE
	// for comparison (not always preserved in live definitions).
	_ = stmt1.Empty
	_ = stmt2.Empty

	// Compare SELECT clauses with formatting tolerance
	if !selectClausesAreEqualWithTolerance(stmt1.AsSelect, stmt2.AsSelect, functions) {
		return false
	}

	return true
}

// refreshClausesAreEqual compares refreshable-MV REFRESH strategies.
// Interval unit plurals are normalized (SECOND vs SECONDS) because ClickHouse
// accepts both spellings via parseIntervalKind.
func refreshClausesAreEqual(a, b *parser.ViewRefreshClause) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if !strings.EqualFold(a.Kind, b.Kind) {
		return false
	}
	if !refreshTimeIntervalsEqual(a.Period, b.Period) {
		return false
	}
	if !refreshTimeIntervalsEqual(a.Offset, b.Offset) {
		return false
	}
	if !refreshTimeIntervalsEqual(a.RandomizeFor, b.RandomizeFor) {
		return false
	}
	if !refreshDependsOnEqual(a.DependsOn, b.DependsOn) {
		return false
	}
	if !refreshSettingsEqual(a.Settings, b.Settings) {
		return false
	}
	if a.Append != b.Append {
		return false
	}
	return true
}

func refreshTimeIntervalsEqual(a, b *parser.RefreshTimeInterval) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a.Parts) != len(b.Parts) {
		return false
	}
	for i := range a.Parts {
		if a.Parts[i].Value != b.Parts[i].Value {
			return false
		}
		if normalizeRefreshUnit(a.Parts[i].Unit) != normalizeRefreshUnit(b.Parts[i].Unit) {
			return false
		}
	}
	return true
}

func normalizeRefreshUnit(unit string) string {
	u := strings.ToUpper(unit)
	switch u {
	case "NANOSECONDS":
		return "NANOSECOND"
	case "MICROSECONDS":
		return "MICROSECOND"
	case "MILLISECONDS":
		return "MILLISECOND"
	case "SECONDS":
		return "SECOND"
	case "MINUTES":
		return "MINUTE"
	case "HOURS":
		return "HOUR"
	case "DAYS":
		return "DAY"
	case "WEEKS":
		return "WEEK"
	case "MONTHS":
		return "MONTH"
	case "QUARTERS":
		return "QUARTER"
	case "YEARS":
		return "YEAR"
	default:
		return u
	}
}

func refreshDependsOnEqual(a, b []parser.ViewRefreshDependency) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(getStringValue(a[i].Database), getStringValue(b[i].Database)) {
			return false
		}
		if !strings.EqualFold(a[i].Name, b[i].Name) {
			return false
		}
	}
	return true
}

func refreshSettingsEqual(a, b *parser.SettingsClause) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a.Values) != len(b.Values) {
		return false
	}
	for i := range a.Values {
		if !strings.EqualFold(a.Values[i].Key, b.Values[i].Key) {
			return false
		}
		if !expressionsAreEqual(&a.Values[i].Value, &b.Values[i].Value) {
			return false
		}
	}
	return true
}

// viewStatementsHaveSameProperties compares statements ignoring names (for rename detection)
func viewStatementsHaveSameProperties(stmt1, stmt2 *parser.CreateViewStmt, functions map[string]*FunctionInfo) bool {
	// For rename detection, we compare everything except the view name
	// This uses the same logic as viewStatementsAreEqual, which ignores creation-time directives
	return viewStatementsAreEqual(stmt1, stmt2, functions)
}

// engineClausesAreEqualWithTolerance compares engine clauses with ClickHouse limitations in mind
func engineClausesAreEqualWithTolerance(engine1, engine2 *parser.ViewEngine) bool {
	// If both are nil, they're equal
	if engine1 == nil && engine2 == nil {
		return true
	}

	// ClickHouse limitation: materialized views don't return ENGINE clause in system.tables
	// If one is nil and the other isn't, this might be a ClickHouse storage limitation
	// For now, we'll be tolerant of this difference
	if engine1 == nil || engine2 == nil {
		// This is likely ClickHouse not preserving ENGINE information
		// We'll allow this difference rather than failing the comparison
		return true
	}

	// Both have values, compare them normally
	return viewEnginesEqual(engine1, engine2)
}

// buildEngineString builds an engine string from ViewEngine struct
// This builds a properly formatted string with spaces for readability
func buildEngineString(engine *parser.ViewEngine) string {
	if engine == nil {
		return ""
	}

	result := engine.Name

	// Add parameters - always add parentheses for engines like MergeTree()
	if len(engine.Parameters) > 0 {
		var params []string
		for _, param := range engine.Parameters {
			params = append(params, param.Value())
		}
		result += "(" + strings.Join(params, ", ") + ")"
	} else {
		// Add empty parentheses for engines like MergeTree()
		result += "()"
	}

	// Add ORDER BY if present (proper format with spaces)
	if engine.OrderBy != nil {
		result += " ORDER BY " + engine.OrderBy.Expression.String()
	}

	// Add PARTITION BY if present (proper format with spaces)
	if engine.PartitionBy != nil {
		result += " PARTITION BY " + engine.PartitionBy.Expression.String()
	}

	// Add PRIMARY KEY if present (proper format with spaces)
	if engine.PrimaryKey != nil {
		result += " PRIMARY KEY " + engine.PrimaryKey.Expression.String()
	}

	// Add SAMPLE BY if present (proper format with spaces)
	if engine.SampleBy != nil {
		result += " SAMPLE BY " + engine.SampleBy.Expression.String()
	}

	return result
}

// viewEnginesEqual compares ViewEngine structures using AST-based comparison
func viewEnginesEqual(engine1, engine2 *parser.ViewEngine) bool {
	if engine1 == nil && engine2 == nil {
		return true
	}
	if engine1 == nil || engine2 == nil {
		return false
	}

	// Compare engine names (case-insensitive)
	if !strings.EqualFold(engine1.Name, engine2.Name) {
		return false
	}

	// Compare parameters
	if len(engine1.Parameters) != len(engine2.Parameters) {
		return false
	}
	for i, param1 := range engine1.Parameters {
		param2 := engine2.Parameters[i]
		if !engineParametersEqual(&param1, &param2) {
			return false
		}
	}

	// Compare ORDER BY clause
	if engine1.OrderBy == nil && engine2.OrderBy == nil {
		// Both nil, continue
	} else if engine1.OrderBy != nil && engine2.OrderBy != nil {
		if !expressionsAreEqual(&engine1.OrderBy.Expression, &engine2.OrderBy.Expression) {
			return false
		}
	} else {
		// One is nil, the other isn't
		return false
	}

	// Compare PARTITION BY clause
	if engine1.PartitionBy == nil && engine2.PartitionBy == nil {
		// Both nil, continue
	} else if engine1.PartitionBy != nil && engine2.PartitionBy != nil {
		if !expressionsAreEqual(&engine1.PartitionBy.Expression, &engine2.PartitionBy.Expression) {
			return false
		}
	} else {
		// One is nil, the other isn't
		return false
	}

	// Compare PRIMARY KEY clause
	if engine1.PrimaryKey == nil && engine2.PrimaryKey == nil {
		// Both nil, continue
	} else if engine1.PrimaryKey != nil && engine2.PrimaryKey != nil {
		if !expressionsAreEqual(&engine1.PrimaryKey.Expression, &engine2.PrimaryKey.Expression) {
			return false
		}
	} else {
		// One is nil, the other isn't
		return false
	}

	// Compare SAMPLE BY clause
	if engine1.SampleBy == nil && engine2.SampleBy == nil {
		// Both nil, continue
	} else if engine1.SampleBy != nil && engine2.SampleBy != nil {
		if !expressionsAreEqual(&engine1.SampleBy.Expression, &engine2.SampleBy.Expression) {
			return false
		}
	} else {
		// One is nil, the other isn't
		return false
	}

	return true
}

// selectClausesAreEqualWithTolerance compares SELECT clauses with formatting tolerance
func selectClausesAreEqualWithTolerance(select1, select2 *parser.SelectStatement, functions map[string]*FunctionInfo) bool {
	if select1 == nil && select2 == nil {
		return true
	}
	if select1 == nil || select2 == nil {
		return false
	}

	// select1 is the live/current side (already reflects ClickHouse's own
	// UDF inlining, if any) and select2 is the source/target side (still
	// spells out UDF calls by name). Inline select2 against the target's own
	// function registry so both sides describe the same expanded form before
	// any AST/normalized comparison runs — see udf_inline.go.
	select2 = inlineFunctionCallsInSelect(select2, functions)

	// First try exact AST comparison
	if selectStatementsAreEqualAST(select1, select2) {
		return true
	}

	// If exact comparison fails, try string-based normalization comparison
	// This handles cases where ClickHouse formatting differs from our parser output
	return selectStatementsAreEqualNormalized(select1, select2)
}

// selectStatementsAreEqualNormalized compares SELECT projections by normalized expression strings.
// Catches semantic renames (e.g. a column renamed from "name" to "id") without looping on ClickHouse SHOW CREATE formatting.
func selectStatementsAreEqualNormalized(stmt1, stmt2 *parser.SelectStatement) bool {
	if (stmt1.With == nil) != (stmt2.With == nil) {
		return false
	}
	if len(stmt1.Columns) != len(stmt2.Columns) {
		return false
	}
	if (stmt1.From == nil) != (stmt2.From == nil) {
		return false
	}
	if (stmt1.Where == nil) != (stmt2.Where == nil) {
		return false
	}
	if (stmt1.GroupBy == nil) != (stmt2.GroupBy == nil) {
		return false
	}
	if (stmt1.Having == nil) != (stmt2.Having == nil) {
		return false
	}
	if (stmt1.OrderBy == nil) != (stmt2.OrderBy == nil) {
		return false
	}
	if (stmt1.Limit == nil) != (stmt2.Limit == nil) {
		return false
	}
	if len(stmt1.Unions) != len(stmt2.Unions) {
		return false
	}

	for i := range stmt1.Columns {
		c1, c2 := stmt1.Columns[i], stmt2.Columns[i]
		if (c1.Star != nil) != (c2.Star != nil) {
			return false
		}
		if normalizeSelectExpr(c1.Expression) != normalizeSelectExpr(c2.Expression) {
			return false
		}
		if normalizeIdent(c1.Alias) != normalizeIdent(c2.Alias) {
			return false
		}
	}

	if !fromClausesAreEqual(stmt1.From, stmt2.From) {
		return false
	}
	if stmt1.Where != nil && stmt2.Where != nil {
		if normalizeSelectExpr(&stmt1.Where.Condition) != normalizeSelectExpr(&stmt2.Where.Condition) {
			return false
		}
	}
	return unionClausesAreEqual(stmt1.Unions, stmt2.Unions)
}

func normalizeSelectExpr(expr *parser.Expression) string {
	if expr == nil {
		return ""
	}
	s := strings.ToLower(expr.String())
	replacer := strings.NewReplacer("`", "", " ", "", "\n", "", "\t", "", "\r", "")
	return replacer.Replace(s)
}

func normalizeIdent(alias *string) string {
	if alias == nil {
		return ""
	}
	return strings.ToLower(strings.Trim(*alias, "`\""))
}

// selectStatementsAreEqualAST compares two SELECT statements using AST-based comparison
func selectStatementsAreEqualAST(stmt1, stmt2 *parser.SelectStatement) bool {
	// Compare WITH clauses (CTEs)
	if !withClausesAreEqual(stmt1.With, stmt2.With) {
		return false
	}

	// Compare SELECT columns
	if !selectColumnsAreEqual(stmt1.Columns, stmt2.Columns) {
		return false
	}

	// Compare FROM clauses
	if !fromClausesAreEqual(stmt1.From, stmt2.From) {
		return false
	}

	// Compare WHERE clauses
	if !whereClausesAreEqual(stmt1.Where, stmt2.Where) {
		return false
	}

	// Compare GROUP BY clauses
	if !groupByClausesAreEqual(stmt1.GroupBy, stmt2.GroupBy) {
		return false
	}

	// Compare HAVING clauses
	if !havingClausesAreEqual(stmt1.Having, stmt2.Having) {
		return false
	}

	// Compare ORDER BY clauses
	if !selectOrderByClausesAreEqual(stmt1.OrderBy, stmt2.OrderBy) {
		return false
	}

	// Compare LIMIT clauses
	if !limitClausesAreEqual(stmt1.Limit, stmt2.Limit) {
		return false
	}

	// Compare SETTINGS clauses
	if !settingsClausesAreEqual(stmt1.Settings, stmt2.Settings) {
		return false
	}

	if !unionClausesAreEqual(stmt1.Unions, stmt2.Unions) {
		return false
	}

	return true
}

func unionClausesAreEqual(u1, u2 []parser.UnionClause) bool {
	if len(u1) != len(u2) {
		return false
	}
	for i := range u1 {
		a, b := u1[i], u2[i]
		if normalizeIdent(a.Mode) != normalizeIdent(b.Mode) {
			return false
		}
		if !withClausesAreEqual(a.With, b.With) {
			return false
		}
		if a.Distinct != b.Distinct {
			return false
		}
		if !selectColumnsAreEqual(a.Columns, b.Columns) {
			return false
		}
		if !fromClausesAreEqual(a.From, b.From) {
			return false
		}
		if !whereClausesAreEqual(a.Where, b.Where) {
			return false
		}
		if !groupByClausesAreEqual(a.GroupBy, b.GroupBy) {
			return false
		}
		if !havingClausesAreEqual(a.Having, b.Having) {
			return false
		}
		if !selectOrderByClausesAreEqual(a.OrderBy, b.OrderBy) {
			return false
		}
		if !limitClausesAreEqual(a.Limit, b.Limit) {
			return false
		}
		if !settingsClausesAreEqual(a.Settings, b.Settings) {
			return false
		}
	}
	return true
}

// withClausesAreEqual compares WITH clauses (named CTEs and expression aliases).
func withClausesAreEqual(with1, with2 *parser.WithClause) bool {
	if eq, done := compare.NilCheck(with1, with2); !done {
		return eq
	}
	return compare.Slices(with1.CTEs, with2.CTEs, commonTableExpressionsAreEqual)
}

func commonTableExpressionsAreEqual(a, b parser.CommonTableExpression) bool {
	if (a.Named == nil) != (b.Named == nil) {
		return false
	}
	if a.Named != nil {
		return normalizeIdentifier(a.Named.Name) == normalizeIdentifier(b.Named.Name) &&
			selectStatementsAreEqualAST(a.Named.Query, b.Named.Query)
	}
	if (a.Expr == nil) != (b.Expr == nil) {
		return false
	}
	if a.Expr == nil {
		return true
	}
	return normalizeIdentifier(a.Expr.Name) == normalizeIdentifier(b.Expr.Name) &&
		expressionsAreEqual(a.Expr.Expression, b.Expr.Expression)
}

// whereClausesAreEqual compares WHERE clauses
func whereClausesAreEqual(where1, where2 *parser.WhereClause) bool {
	if eq, done := compare.NilCheck(where1, where2); !done {
		return eq
	}
	return expressionsAreEqual(&where1.Condition, &where2.Condition)
}

// havingClausesAreEqual compares HAVING clauses
func havingClausesAreEqual(having1, having2 *parser.HavingClause) bool {
	if eq, done := compare.NilCheck(having1, having2); !done {
		return eq
	}
	return expressionsAreEqual(&having1.Condition, &having2.Condition)
}

// selectOrderByClausesAreEqual compares SELECT ORDER BY clauses
func selectOrderByClausesAreEqual(order1, order2 *parser.SelectOrderByClause) bool {
	if eq, done := compare.NilCheck(order1, order2); !done {
		return eq
	}
	return compare.Slices(order1.Columns, order2.Columns, func(a, b parser.OrderByColumn) bool {
		return orderByColumnsAreEqual(&a, &b)
	})
}

// orderByColumnsAreEqual compares ORDER BY columns
func orderByColumnsAreEqual(col1, col2 *parser.OrderByColumn) bool {
	if !expressionsAreEqual(&col1.Expression, &col2.Expression) {
		return false
	}

	// Compare direction (ASC/DESC) with normalization
	dir1 := ""
	if col1.Direction != nil {
		dir1 = strings.ToUpper(*col1.Direction)
	}
	dir2 := ""
	if col2.Direction != nil {
		dir2 = strings.ToUpper(*col2.Direction)
	}

	// Compare NULLS FIRST/LAST with normalization
	nulls1 := ""
	if col1.Nulls != nil {
		nulls1 = strings.ToUpper(*col1.Nulls)
	}
	nulls2 := ""
	if col2.Nulls != nil {
		nulls2 = strings.ToUpper(*col2.Nulls)
	}

	return dir1 == dir2 && nulls1 == nulls2
}

// selectColumnsAreEqual compares SELECT column lists
func selectColumnsAreEqual(cols1, cols2 []parser.SelectColumn) bool {
	if len(cols1) != len(cols2) {
		return false
	}
	for i, col1 := range cols1 {
		col2 := cols2[i]
		if !expressionsAreEqual(col1.Expression, col2.Expression) {
			return false
		}
		// Compare aliases (normalize case and handle nil)
		alias1 := ""
		if col1.Alias != nil {
			alias1 = normalizeIdentifier(*col1.Alias)
		}
		alias2 := ""
		if col2.Alias != nil {
			alias2 = normalizeIdentifier(*col2.Alias)
		}
		if alias1 != alias2 {
			return false
		}
	}
	return true
}

// fromClausesAreEqual compares FROM clauses
func fromClausesAreEqual(from1, from2 *parser.FromClause) bool {
	if eq, done := compare.NilCheck(from1, from2); !done {
		return eq
	}

	return tableRefsAreEqual(&from1.Table, &from2.Table) &&
		compare.Slices(from1.Joins, from2.Joins, func(a, b parser.JoinClause) bool {
			return joinsAreEqual(&a, &b)
		})
}

// tableRefsAreEqual compares table references
func tableRefsAreEqual(table1, table2 *parser.TableRef) bool {
	if eq, done := compare.NilCheck(table1, table2); !done {
		return eq
	}

	// Compare table names
	if table1.TableName != nil && table2.TableName != nil {
		return tableNamesWithAliasAreEqual(table1.TableName, table2.TableName)
	}
	if table1.TableName != nil || table2.TableName != nil {
		return false
	}

	// Compare subqueries
	if table1.Subquery != nil && table2.Subquery != nil {
		return subqueriesWithAliasAreEqual(table1.Subquery, table2.Subquery)
	}
	if table1.Subquery != nil || table2.Subquery != nil {
		return false
	}

	// Compare function calls
	if table1.Function != nil && table2.Function != nil {
		return functionsWithAliasAreEqual(table1.Function, table2.Function)
	}
	return table1.Function == nil && table2.Function == nil
}

// tableNamesWithAliasAreEqual compares table names with aliases
func tableNamesWithAliasAreEqual(name1, name2 *parser.TableNameWithAlias) bool {
	if eq, done := compare.NilCheck(name1, name2); !done {
		return eq
	}

	// Compare database names (ClickHouse dump often qualifies default.t)
	db1, db2 := "", ""
	if name1.Database != nil {
		db1 = normalizeDefaultDatabase(*name1.Database)
	}
	if name2.Database != nil {
		db2 = normalizeDefaultDatabase(*name2.Database)
	}
	if db1 != db2 {
		return false
	}

	// Compare table names
	if normalizeIdentifier(name1.Table) != normalizeIdentifier(name2.Table) {
		return false
	}

	// Compare aliases - for now, just check if both have or don't have aliases
	hasAlias1 := name1.ExplicitAlias != nil || name1.ImplicitAlias != nil
	hasAlias2 := name2.ExplicitAlias != nil || name2.ImplicitAlias != nil
	return hasAlias1 == hasAlias2
}

// subqueriesWithAliasAreEqual compares subqueries with aliases
func subqueriesWithAliasAreEqual(sub1, sub2 *parser.SubqueryWithAlias) bool {
	if sub1 == nil && sub2 == nil {
		return true
	}
	if sub1 == nil || sub2 == nil {
		return false
	}

	// Compare the actual subqueries
	if !selectStatementsAreEqualAST(&sub1.Subquery, &sub2.Subquery) {
		return false
	}

	// Compare aliases
	alias1 := ""
	if sub1.Alias != nil {
		alias1 = normalizeIdentifier(*sub1.Alias)
	}
	alias2 := ""
	if sub2.Alias != nil {
		alias2 = normalizeIdentifier(*sub2.Alias)
	}
	return alias1 == alias2
}

// functionsWithAliasAreEqual compares function calls with aliases using AST-based comparison
func functionsWithAliasAreEqual(fn1, fn2 *parser.FunctionWithAlias) bool {
	if fn1 == nil && fn2 == nil {
		return true
	}
	if fn1 == nil || fn2 == nil {
		return false
	}

	// Compare function names (case-insensitive for ClickHouse)
	if !strings.EqualFold(fn1.Function.Name, fn2.Function.Name) {
		return false
	}

	// Compare argument lists
	if len(fn1.Function.Arguments) != len(fn2.Function.Arguments) {
		return false
	}

	for i, arg1 := range fn1.Function.Arguments {
		arg2 := fn2.Function.Arguments[i]
		if !functionArgsEqual(&arg1, &arg2) {
			return false
		}
	}

	// Compare aliases (case-insensitive)
	if fn1.Alias == nil && fn2.Alias == nil {
		return true
	}
	if fn1.Alias == nil || fn2.Alias == nil {
		return false
	}
	return strings.EqualFold(*fn1.Alias, *fn2.Alias)
}

// joinsAreEqual compares JOIN clauses
func joinsAreEqual(join1, join2 *parser.JoinClause) bool {
	if join1.Strictness != join2.Strictness || join1.Type != join2.Type || join1.Join != join2.Join {
		return false
	}
	if !tableRefsAreEqual(&join1.Table, &join2.Table) {
		return false
	}

	// Compare join conditions
	if join1.Condition == nil && join2.Condition == nil {
		return true
	}
	if join1.Condition == nil || join2.Condition == nil {
		return false
	}

	// Compare ON conditions
	if join1.Condition.On != nil && join2.Condition.On != nil {
		return expressionsAreEqual(join1.Condition.On, join2.Condition.On)
	}
	if join1.Condition.On != nil || join2.Condition.On != nil {
		return false
	}

	// Compare USING conditions
	if len(join1.Condition.Using) != len(join2.Condition.Using) {
		return false
	}
	for i, col1 := range join1.Condition.Using {
		col2 := join2.Condition.Using[i]
		if normalizeIdentifier(col1) != normalizeIdentifier(col2) {
			return false
		}
	}

	return true
}

// groupByClausesAreEqual compares GROUP BY clauses
func groupByClausesAreEqual(group1, group2 *parser.GroupByClause) bool {
	if group1 == nil && group2 == nil {
		return true
	}
	if group1 == nil || group2 == nil {
		return false
	}
	if len(group1.Columns) != len(group2.Columns) {
		return false
	}
	for i, expr1 := range group1.Columns {
		expr2 := group2.Columns[i]
		if !expressionsAreEqual(&expr1, &expr2) {
			return false
		}
	}
	return true
}

// limitClausesAreEqual compares LIMIT clauses
func limitClausesAreEqual(limit1, limit2 *parser.LimitClause) bool {
	if limit1 == nil && limit2 == nil {
		return true
	}
	if limit1 == nil || limit2 == nil {
		return false
	}

	// Compare count expressions
	if !expressionsAreEqual(&limit1.Count, &limit2.Count) {
		return false
	}

	// Compare offset expressions
	if limit1.Offset == nil && limit2.Offset == nil {
		return true
	}
	if limit1.Offset == nil || limit2.Offset == nil {
		return false
	}
	return expressionsAreEqual(&limit1.Offset.Value, &limit2.Offset.Value)
}

// settingsClausesAreEqual compares SETTINGS clauses
func settingsClausesAreEqual(settings1, settings2 *parser.SettingsClause) bool {
	if settings1 == nil && settings2 == nil {
		return true
	}
	if settings1 == nil || settings2 == nil {
		return false
	}
	if len(settings1.Values) != len(settings2.Values) {
		return false
	}

	// Create maps for easier comparison
	map1 := make(map[string]string)
	for _, setting := range settings1.Values {
		map1[normalizeIdentifier(setting.Key)] = setting.Value.String()
	}

	map2 := make(map[string]string)
	for _, setting := range settings2.Values {
		map2[normalizeIdentifier(setting.Key)] = setting.Value.String()
	}

	// Compare maps
	for name, value1 := range map1 {
		value2, exists := map2[name]
		if !exists || value1 != value2 {
			return false
		}
	}

	return true
}

// selectStatementToString converts a SelectStatement to a properly formatted string representation
func selectStatementToString(stmt *parser.SelectStatement) string {
	if stmt == nil {
		return ""
	}

	// The formatSelectStatement method is private, so we'll use a workaround
	// Create a fake SELECT statement wrapper and format it, then extract the SELECT part
	fakeSQL := &parser.SQL{
		Statements: []*parser.Statement{
			{
				SelectStatement: &parser.TopLevelSelectStatement{
					SelectStatement: *stmt,
					Semicolon:       true,
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := format.FormatSQL(&buf, format.Defaults, fakeSQL); err != nil {
		// Fallback to basic string representation if formatting fails
		return "SELECT * FROM unknown"
	}

	// Extract the formatted SELECT statement
	formatted := buf.String()
	// Remove any trailing semicolon since this is used within CREATE VIEW statements
	formatted = strings.TrimSuffix(formatted, ";")
	// Convert to single line for view definitions
	formatted = strings.ReplaceAll(formatted, "\n", " ")
	// Clean up multiple spaces
	formatted = strings.Join(strings.Fields(formatted), " ")

	return formatted
}

// getViewType returns a human-readable view type string
func getViewType(view *ViewInfo) string {
	if view.IsMaterialized {
		return "materialized view"
	}
	return "view"
}

// getFullViewName returns the full name of a view (database.name or just name)
func getFullViewName(view *ViewInfo) string {
	if view.Database != "" {
		return view.Database + "." + view.Name
	}
	return view.Name
}

// generateCreateViewSQL generates CREATE VIEW SQL from ViewInfo
func generateCreateViewSQL(view *ViewInfo) string {
	sql := "CREATE"

	if view.OrReplace {
		sql += " OR REPLACE"
	}

	if view.IsMaterialized {
		sql += " MATERIALIZED"
	}

	sql += " VIEW"

	if view.Statement.IfNotExists {
		sql += " IF NOT EXISTS"
	}

	sql += " " + getFullViewName(view)

	if view.Cluster != "" {
		sql += " ON CLUSTER " + view.Cluster
	}

	if view.Statement.Refresh != nil {
		sql += " " + buildRefreshClauseString(view.Statement.Refresh)
	}

	toValue := getViewTableTargetValue(view.Statement.To)
	if toValue != "" {
		sql += " TO " + toValue
	}

	if view.Statement.Engine != nil {
		sql += " ENGINE = " + buildEngineString(view.Statement.Engine)
	}

	if view.Statement.Populate {
		sql += " POPULATE"
	}

	if view.Statement.Empty {
		sql += " EMPTY"
	}

	if view.Statement.AsSelect != nil {
		sql += " AS " + selectStatementToString(view.Statement.AsSelect)
	}

	return sql + ";"
}

// buildRefreshClauseString renders a REFRESH strategy for migration SQL.
func buildRefreshClauseString(refresh *parser.ViewRefreshClause) string {
	if refresh == nil {
		return ""
	}

	parts := []string{"REFRESH"}

	if refresh.Kind != "" {
		parts = append(parts, refresh.Kind)
		if refresh.Period != nil {
			parts = append(parts, buildRefreshTimeIntervalString(refresh.Period))
		}
		if refresh.Offset != nil {
			parts = append(parts, "OFFSET", buildRefreshTimeIntervalString(refresh.Offset))
		}
	}

	if refresh.RandomizeFor != nil {
		parts = append(parts, "RANDOMIZE FOR", buildRefreshTimeIntervalString(refresh.RandomizeFor))
	}

	if len(refresh.DependsOn) > 0 {
		deps := make([]string, 0, len(refresh.DependsOn))
		for _, dep := range refresh.DependsOn {
			if dep.Database != nil && *dep.Database != "" {
				deps = append(deps, *dep.Database+"."+dep.Name)
			} else {
				deps = append(deps, dep.Name)
			}
		}
		parts = append(parts, "DEPENDS ON", strings.Join(deps, ", "))
	}

	if refresh.Settings != nil && len(refresh.Settings.Values) > 0 {
		settings := make([]string, 0, len(refresh.Settings.Values))
		for _, assignment := range refresh.Settings.Values {
			settings = append(settings, assignment.Key+" = "+assignment.Value.String())
		}
		parts = append(parts, "SETTINGS", strings.Join(settings, ", "))
	}

	if refresh.Append {
		parts = append(parts, "APPEND")
	}

	return strings.Join(parts, " ")
}

func buildRefreshTimeIntervalString(interval *parser.RefreshTimeInterval) string {
	if interval == nil || len(interval.Parts) == 0 {
		return ""
	}
	pieces := make([]string, 0, len(interval.Parts)*2)
	for _, part := range interval.Parts {
		pieces = append(pieces, part.Value, part.Unit)
	}
	return strings.Join(pieces, " ")
}

// generateDropViewSQL generates DROP VIEW/TABLE SQL from ViewInfo
func generateDropViewSQL(view *ViewInfo) string {
	var database *string
	if view.Database != "" {
		database = &view.Database
	}

	objectType := "VIEW"
	if view.IsMaterialized {
		// Materialized views are dropped using DROP TABLE
		objectType = "TABLE"
	}

	return utils.NewSQLBuilder().
		Drop(objectType).
		IfExists().
		QualifiedName(database, view.Name).
		OnCluster(view.Cluster).
		String()
}

// generateCreateOrReplaceViewSQL generates CREATE OR REPLACE VIEW SQL for regular views
func generateCreateOrReplaceViewSQL(view *ViewInfo) string {
	sql := "CREATE OR REPLACE VIEW " + getFullViewName(view)

	if view.Cluster != "" {
		sql += " ON CLUSTER " + view.Cluster
	}

	if view.Statement.AsSelect != nil {
		sql += " AS " + selectStatementToString(view.Statement.AsSelect)
	}

	return sql + ";"
}

// generateRenameViewSQL generates RENAME TABLE SQL for both regular and materialized views
func generateRenameViewSQL(from, to *ViewInfo) string {
	var fromDB, toDB *string
	if from.Database != "" {
		fromDB = &from.Database
	}
	if to.Database != "" {
		toDB = &to.Database
	}

	return utils.NewSQLBuilder().
		Rename("TABLE").
		QualifiedName(fromDB, from.Name).
		QualifiedTo(toDB, to.Name).
		OnCluster(to.Cluster).
		String()
}
