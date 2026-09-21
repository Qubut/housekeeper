package clickhouse

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/pseudomuto/housekeeper/pkg/parser"
)

var (
	asSelectClause     = regexp.MustCompile(`(?i)\sAS\s+SELECT\b`)
	definerSQLSecurity = regexp.MustCompile(`(?i)\s*DEFINER\s*=\s*\S+\s+SQL\s+SECURITY\s+\S+`)
)

// stripDumpedViewHeader removes ClickHouse create_table_query extras the
// grammar does not accept: a stored column list after the view name, and
// DEFINER / SQL SECURITY. REFRESH, TO, ENGINE, and AS SELECT stay so a
// dumped refreshable materialized view still compares as the same object.
func stripDumpedViewHeader(query string) string {
	loc := asSelectClause.FindStringIndex(query)
	if loc == nil {
		return query
	}
	header := query[:loc[0]]
	rest := query[loc[0]:]
	header = stripViewColumnList(header)
	header = definerSQLSecurity.ReplaceAllString(header, " ")
	header = strings.TrimSpace(header)
	if header == "" {
		return query
	}
	return header + rest
}

func stripViewColumnList(header string) string {
	start, end := lastBalancedParenGroup(header)
	if start < 0 || !looksLikeColumnList(header[start:end+1]) {
		return header
	}
	return strings.TrimSpace(header[:start]) + " " + strings.TrimSpace(header[end+1:])
}

func lastBalancedParenGroup(s string) (int, int) {
	end := strings.LastIndex(s, ")")
	if end < 0 {
		return -1, -1
	}
	depth := 0
	for i := end; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return i, end
			}
		}
	}
	return -1, -1
}

func looksLikeColumnList(group string) bool {
	lower := strings.ToLower(group)
	markers := []string{
		" string", " uint64", " uint32", " int64", " int32",
		" datetime", " date", " float64", " float32", " bool", " uuid",
		"lowcardinality", "nullable(", " array(", " decimal",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func stripThroughAsSelect(query string) string {
	loc := asSelectClause.FindStringIndex(query)
	if loc == nil {
		return query
	}
	viewNameEnd := strings.Index(query, "(")
	if viewNameEnd > 0 && loc[0] > viewNameEnd {
		return query[:viewNameEnd] + query[loc[0]:]
	}
	return query
}

// extractViews retrieves all view definitions (both regular and materialized) from the ClickHouse instance.
// This function queries the system.tables table to get complete view information and returns them
// as parsed DDL statements, handling both regular views and materialized views.
//
// System views are automatically excluded. All DDL statements are validated
// using the parser before being returned.
//
// Example:
//
//	client, err := clickhouse.NewClient(ctx, "localhost:9000")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer client.Close()
//
//	views, err := client.GetViews(ctx)
//	if err != nil {
//		log.Fatalf("Failed to extract views: %v", err)
//	}
//
//	// Process the parsed view statements
//	for _, stmt := range views.Statements {
//		if stmt.CreateView != nil {
//			viewType := "VIEW"
//			if stmt.CreateView.Materialized {
//				viewType = "MATERIALIZED VIEW"
//			}
//			name := stmt.CreateView.Name
//			if stmt.CreateView.Database != nil {
//				name = *stmt.CreateView.Database + "." + name
//			}
//			fmt.Printf("%s: %s\n", viewType, name)
//		}
//	}
//
// Returns a *parser.SQL containing view CREATE statements or an error if extraction fails.
func extractViews(ctx context.Context, client *Client) (*parser.SQL, error) {
	condition, params := buildDatabaseExclusion("database", client.options.IgnoreDatabases)
	query := fmt.Sprintf(`
		SELECT 
			create_table_query
		FROM system.tables
		WHERE %s
		  AND engine IN ('View', 'MaterializedView')
		ORDER BY database, name
	`, condition)

	rows, err := client.conn.Query(ctx, query, params...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query views")
	}
	defer rows.Close()

	var statements []string
	for rows.Next() {
		var createQuery string
		if err := rows.Scan(&createQuery); err != nil {
			return nil, errors.Wrap(err, "failed to scan view row")
		}

		cleanedQuery := cleanCreateStatement(createQuery)
		precise := stripDumpedViewHeader(cleanedQuery)
		if err := validateDDLStatement(precise); err == nil {
			cleanedQuery = precise
		} else {
			cleanedQuery = stripThroughAsSelect(cleanedQuery)
			if err := validateDDLStatement(cleanedQuery); err != nil {
				return nil, errors.Wrapf(err, "generated invalid DDL for view (query: %s)", precise)
			}
		}

		statements = append(statements, cleanedQuery)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "error iterating view rows")
	}

	// Parse all statements into a SQL structure
	combinedSQL := strings.Join(statements, "\n")

	sqlResult, err := parser.ParseString(combinedSQL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse combined view DDL")
	}

	return sqlResult, nil
}
