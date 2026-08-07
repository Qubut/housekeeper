package parser

type (
	// ViewTableTarget represents a table target in TO clause of materialized view
	// Can be either:
	//   - Simple table reference: [db.]table_name
	//   - Table function: functionName(args...)
	ViewTableTarget struct {
		// Try table function first (has parentheses to distinguish it)
		Function *TableFunction `parser:"@@"`
		// Fall back to table reference if no function call syntax found
		Database *string `parser:"| ((@(Ident | BacktickIdent) '.')?"`
		Table    *string `parser:"@(Ident | BacktickIdent))"`
	}

	// RefreshIntervalPart is one `N UNIT` component of a refresh time interval
	// (e.g. `10 SECOND`, `2 DAY`). Singular and plural unit keywords are accepted
	// to match ClickHouse's parseIntervalKind.
	RefreshIntervalPart struct {
		Value string `parser:"@Number"`
		Unit  string `parser:"@('NANOSECOND' | 'NANOSECONDS' | 'MICROSECOND' | 'MICROSECONDS' | 'MILLISECOND' | 'MILLISECONDS' | 'SECOND' | 'SECONDS' | 'MINUTE' | 'MINUTES' | 'HOUR' | 'HOURS' | 'DAY' | 'DAYS' | 'WEEK' | 'WEEKS' | 'MONTH' | 'MONTHS' | 'QUARTER' | 'QUARTERS' | 'YEAR' | 'YEARS')"`
	}

	// RefreshTimeInterval is one or more RefreshIntervalPart values, matching
	// ClickHouse ParserTimeInterval (e.g. `1 WEEK`, `2 DAY 3 HOUR`).
	RefreshTimeInterval struct {
		Parts []RefreshIntervalPart `parser:"@@+"`
	}

	// ViewRefreshDependency is a `[db.]name` entry in DEPENDS ON.
	ViewRefreshDependency struct {
		Database *string `parser:"(@(Ident | BacktickIdent) '.')?"`
		Name     string  `parser:"@(Ident | BacktickIdent)"`
	}

	// ViewRefreshClause represents the REFRESH strategy on a refreshable
	// materialized view. Mirrors ClickHouse ParserRefreshStrategy: schedule
	// (EVERY/AFTER), optional RANDOMIZE FOR / DEPENDS ON / SETTINGS, then APPEND.
	// CREATE places this clause after ON CLUSTER and before TO / ENGINE.
	ViewRefreshClause struct {
		Refresh      string                  `parser:"'REFRESH'"`
		Kind         string                  `parser:"@('EVERY' | 'AFTER')?"`
		Period       *RefreshTimeInterval    `parser:"@@?"`
		Offset       *RefreshTimeInterval    `parser:"('OFFSET' @@)?"`
		RandomizeFor *RefreshTimeInterval    `parser:"('RANDOMIZE' 'FOR' @@)?"`
		DependsOn    []ViewRefreshDependency `parser:"('DEPENDS' 'ON' @@ (',' @@)*)?"`
		Settings     *SettingsClause         `parser:"@@?"`
		Append       bool                    `parser:"@'APPEND'?"`
	}

	// CreateViewStmt represents a CREATE VIEW statement.
	// Supports both regular views and materialized views (including refreshable).
	// ClickHouse syntax:
	//   CREATE [OR REPLACE] [MATERIALIZED] VIEW [IF NOT EXISTS] [db.]view_name [ON CLUSTER cluster]
	//   [REFRESH [EVERY|AFTER interval [OFFSET interval]] [RANDOMIZE FOR interval]
	//            [DEPENDS ON ...] [SETTINGS ...] [APPEND]]
	//   [TO [db.]table_name] [ENGINE = engine] [POPULATE | EMPTY]
	//   AS SELECT ...
	CreateViewStmt struct {
		LeadingCommentField
		Create       string             `parser:"'CREATE'"`
		OrReplace    bool               `parser:"@('OR' 'REPLACE')?"`
		Materialized bool               `parser:"@'MATERIALIZED'?"`
		View         string             `parser:"'VIEW'"`
		IfNotExists  bool               `parser:"@('IF' 'NOT' 'EXISTS')?"`
		Database     *string            `parser:"(@(Ident | BacktickIdent) '.')?"`
		Name         string             `parser:"@(Ident | BacktickIdent)"`
		OnCluster    *string            `parser:"('ON' 'CLUSTER' @(Ident | BacktickIdent | String))?"`
		Refresh      *ViewRefreshClause `parser:"@@?"`
		To           *ViewTableTarget   `parser:"('TO' @@)?"`
		Engine       *ViewEngine        `parser:"@@?"`
		Populate     bool               `parser:"@'POPULATE'?"`
		Empty        bool               `parser:"@'EMPTY'?"`
		AsSelect     *SelectStatement   `parser:"'AS' @@"`
		TrailingCommentField
		Semicolon bool `parser:"';'"`
	}

	// AttachViewStmt represents an ATTACH VIEW statement (for regular views only).
	// ClickHouse syntax:
	//   ATTACH VIEW [IF NOT EXISTS] [db.]view_name [ON CLUSTER cluster]
	AttachViewStmt struct {
		LeadingCommentField
		Attach      string  `parser:"'ATTACH'"`
		View        string  `parser:"'VIEW'"`
		IfNotExists bool    `parser:"@('IF' 'NOT' 'EXISTS')?"`
		Database    *string `parser:"(@(Ident | BacktickIdent) '.')?"`
		Name        string  `parser:"@(Ident | BacktickIdent)"`
		OnCluster   *string `parser:"('ON' 'CLUSTER' @(Ident | BacktickIdent | String))?"`
		TrailingCommentField
		Semicolon bool `parser:"';'"`
	}

	// DetachViewStmt represents a DETACH VIEW statement (for regular views only).
	// ClickHouse syntax:
	//   DETACH VIEW [IF EXISTS] [db.]view_name [ON CLUSTER cluster] [PERMANENTLY] [SYNC]
	DetachViewStmt struct {
		LeadingCommentField
		Detach      string  `parser:"'DETACH'"`
		View        string  `parser:"'VIEW'"`
		IfExists    bool    `parser:"@('IF' 'EXISTS')?"`
		Database    *string `parser:"(@(Ident | BacktickIdent) '.')?"`
		Name        string  `parser:"@(Ident | BacktickIdent)"`
		OnCluster   *string `parser:"('ON' 'CLUSTER' @(Ident | BacktickIdent | String))?"`
		Permanently bool    `parser:"@'PERMANENTLY'?"`
		Sync        bool    `parser:"@'SYNC'?"`
		TrailingCommentField
		Semicolon bool `parser:"';'"`
	}

	// DropViewStmt represents a DROP VIEW statement (for regular views only).
	// ClickHouse syntax:
	//   DROP VIEW [IF EXISTS] [db.]view_name [ON CLUSTER cluster] [SYNC]
	DropViewStmt struct {
		LeadingCommentField
		Drop      string  `parser:"'DROP'"`
		View      string  `parser:"'VIEW'"`
		IfExists  bool    `parser:"@('IF' 'EXISTS')?"`
		Database  *string `parser:"(@(Ident | BacktickIdent) '.')?"`
		Name      string  `parser:"@(Ident | BacktickIdent)"`
		OnCluster *string `parser:"('ON' 'CLUSTER' @(Ident | BacktickIdent | String))?"`
		Sync      bool    `parser:"@'SYNC'?"`
		TrailingCommentField
		Semicolon bool `parser:"';'"`
	}

	// ViewEngine represents ENGINE = clause for materialized views.
	// Materialized views can have ENGINE clauses with additional DDL like ORDER BY.
	// We structure this similar to table engines but with optional materialized view specific clauses.
	ViewEngine struct {
		Engine      string            `parser:"'ENGINE' '='"`
		Name        string            `parser:"@(Ident | BacktickIdent)"`
		Parameters  []EngineParameter `parser:"('(' (@@ (',' @@)*)? ')')?"`
		OrderBy     *ViewOrderBy      `parser:"@@?"`
		PartitionBy *ViewPartitionBy  `parser:"@@?"`
		PrimaryKey  *ViewPrimaryKey   `parser:"@@?"`
		SampleBy    *ViewSampleBy     `parser:"@@?"`
	}

	// ViewOrderBy represents ORDER BY in materialized view ENGINE clause
	ViewOrderBy struct {
		OrderBy    string     `parser:"'ORDER' 'BY'"`
		Expression Expression `parser:"@@"`
	}

	// ViewPartitionBy represents PARTITION BY in materialized view ENGINE clause
	ViewPartitionBy struct {
		PartitionBy string     `parser:"'PARTITION' 'BY'"`
		Expression  Expression `parser:"@@"`
	}

	// ViewPrimaryKey represents PRIMARY KEY in materialized view ENGINE clause
	ViewPrimaryKey struct {
		PrimaryKey string     `parser:"'PRIMARY' 'KEY'"`
		Expression Expression `parser:"@@"`
	}

	// ViewSampleBy represents SAMPLE BY in materialized view ENGINE clause
	ViewSampleBy struct {
		SampleBy   string     `parser:"'SAMPLE' 'BY'"`
		Expression Expression `parser:"@@"`
	}
)
