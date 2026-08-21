package parser

// This file contains comprehensive ClickHouse query parsing structures including SELECT statements

type (
	// SelectStatement represents a SELECT statement (for subqueries, no semicolon).
	// Optional Unions holds trailing UNION [ALL|DISTINCT] SELECT arms (ClickHouse
	// set-ops). WITH applies only to the first arm; each UnionClause is its own
	// SELECT ... FROM ... chain.
	SelectStatement struct {
		With     *WithClause          `parser:"@@?"`
		Select   string               `parser:"'SELECT'"`
		Distinct bool                 `parser:"@'DISTINCT'?"`
		Columns  []SelectColumn       `parser:"@@ (',' @@)*"`
		From     *FromClause          `parser:"@@?"`
		Where    *WhereClause         `parser:"@@?"`
		GroupBy  *GroupByClause       `parser:"@@?"`
		Having   *HavingClause        `parser:"@@?"`
		OrderBy  *SelectOrderByClause `parser:"@@?"`
		Limit    *LimitClause         `parser:"@@?"`
		Settings *SettingsClause      `parser:"@@?"`
		Unions   []UnionClause        `parser:"@@*"`
	}

	// UnionClause is one UNION [ALL|DISTINCT] SELECT arm after the first select.
	// Mode is "ALL", "DISTINCT", or nil (bare UNION; ClickHouse uses union_default_mode).
	UnionClause struct {
		Mode     *string              `parser:"'UNION' @('ALL' | 'DISTINCT')?"`
		Select   string               `parser:"'SELECT'"`
		Distinct bool                 `parser:"@'DISTINCT'?"`
		Columns  []SelectColumn       `parser:"@@ (',' @@)*"`
		From     *FromClause          `parser:"@@?"`
		Where    *WhereClause         `parser:"@@?"`
		GroupBy  *GroupByClause       `parser:"@@?"`
		Having   *HavingClause        `parser:"@@?"`
		OrderBy  *SelectOrderByClause `parser:"@@?"`
		Limit    *LimitClause         `parser:"@@?"`
		Settings *SettingsClause      `parser:"@@?"`
	}

	// TopLevelSelectStatement represents a top-level SELECT statement (requires semicolon)
	TopLevelSelectStatement struct {
		SelectStatement
		Semicolon bool `parser:"';'"`
	}

	// WithClause represents a WITH clause: named CTEs and/or expression aliases.
	WithClause struct {
		With string                  `parser:"'WITH'"`
		CTEs []CommonTableExpression `parser:"@@ (',' @@)*"`
	}

	// CommonTableExpression is one WITH binding.
	// Named CTE form (`name AS (SELECT ...)`) is tried before expression-alias
	// form (`expr AS name`) so identifiers are not consumed as bare expressions.
	CommonTableExpression struct {
		Named *NamedCommonTableExpression `parser:"@@"`
		Expr  *WithExpressionAlias        `parser:"| @@"`
	}

	// NamedCommonTableExpression is `identifier AS (subquery)`.
	NamedCommonTableExpression struct {
		Name  string           `parser:"@(Ident | BacktickIdent)"`
		As    string           `parser:"'AS'"`
		Query *SelectStatement `parser:"'(' @@ ')'"`
	}

	// WithExpressionAlias is ClickHouse `expression AS identifier`
	// (including scalar subqueries used as LIMIT bounds).
	WithExpressionAlias struct {
		Expression *Expression `parser:"@@"`
		As         string      `parser:"'AS'"`
		Name       string      `parser:"@(Ident | BacktickIdent)"`
	}

	// SelectColumn represents a column in SELECT clause
	SelectColumn struct {
		Star       *string     `parser:"@'*'"`
		Expression *Expression `parser:"| @@"`
		Alias      *string     `parser:"('AS' @(Ident | BacktickIdent))?"`
	}

	// FromClause represents FROM clause with joins
	FromClause struct {
		From  string       `parser:"'FROM'"`
		Table TableRef     `parser:"@@"`
		Joins []JoinClause `parser:"@@*"`
	}

	// TableRef represents a table reference (table, subquery, or function)
	TableRef struct {
		// Try table function first (has parentheses to distinguish it)
		Function *FunctionWithAlias `parser:"@@"`
		// OR subquery with optional alias
		Subquery *SubqueryWithAlias `parser:"| @@"`
		// Fall back to table reference if no function call syntax found
		TableName *TableNameWithAlias `parser:"| @@"`
	}

	// TableNameWithAlias represents a table name with optional alias
	TableNameWithAlias struct {
		Database      *string     `parser:"(@(Ident | BacktickIdent) '.')?"`
		Table         string      `parser:"@(Ident | BacktickIdent)"`
		ExplicitAlias *TableAlias `parser:"@@?"`
		ImplicitAlias *string     `parser:"| @(Ident | BacktickIdent)"`
	}

	// TableAlias represents a table alias - requires explicit AS keyword
	TableAlias struct {
		Name *string `parser:"'AS' @(Ident | BacktickIdent)"`
	}

	// SubqueryWithAlias represents a subquery with optional alias
	SubqueryWithAlias struct {
		Subquery SelectStatement `parser:"'(' @@ ')'"`
		Alias    *string         `parser:"('AS' @(Ident | BacktickIdent))?"`
	}

	// FunctionWithAlias represents a table function with optional alias
	FunctionWithAlias struct {
		Function TableFunction `parser:"@@"`
		Alias    *string       `parser:"('AS' @(Ident | BacktickIdent))?"`
	}

	// TableFunction represents table functions like numbers(), remote(), etc.
	TableFunction struct {
		Name      string        `parser:"@(Ident | BacktickIdent)"`
		Arguments []FunctionArg `parser:"'(' (@@ (',' @@)*)? ')'"`
	}

	// JoinClause represents JOIN operations.
	// ClickHouse order: [ASOF|ANY|ALL] [INNER|LEFT|…] JOIN (ASOF before join type).
	JoinClause struct {
		Strictness string         `parser:"@('ASOF' | 'ANY' | 'ALL')?"`
		Type       string         `parser:"@('INNER' | 'LEFT' | 'RIGHT' | 'FULL' | 'CROSS')?"`
		Join       string         `parser:"@('JOIN' | 'ARRAY' 'JOIN' | 'GLOBAL' 'JOIN')"`
		Table      TableRef       `parser:"@@"`
		Condition  *JoinCondition `parser:"@@?"`
	}

	// JoinCondition represents ON or USING clause in joins
	JoinCondition struct {
		On    *Expression `parser:"'ON' @@"`
		Using []string    `parser:"| 'USING' '(' @(Ident | BacktickIdent) (',' @(Ident | BacktickIdent))* ')'"`
	}

	// WhereClause represents WHERE clause
	WhereClause struct {
		Where     string     `parser:"'WHERE'"`
		Condition Expression `parser:"@@"`
	}

	// GroupByClause represents GROUP BY clause
	GroupByClause struct {
		GroupBy    string       `parser:"'GROUP' 'BY'"`
		All        bool         `parser:"(@'ALL'"`
		Columns    []Expression `parser:"| @@ (',' @@)*)"`
		WithClause *string      `parser:"('WITH' @('CUBE' | 'ROLLUP' | 'TOTALS'))?"`
	}

	// HavingClause represents HAVING clause
	HavingClause struct {
		Having    string     `parser:"'HAVING'"`
		Condition Expression `parser:"@@"`
	}

	// SelectOrderByClause represents ORDER BY clause in SELECT statements
	SelectOrderByClause struct {
		OrderBy     string             `parser:"'ORDER' 'BY'"`
		Columns     []OrderByColumn    `parser:"@@ (',' @@)*"`
		Interpolate *InterpolateClause `parser:"@@?"`
	}

	// InterpolateClause represents INTERPOLATE clause for WITH FILL
	InterpolateClause struct {
		Interpolate string              `parser:"'INTERPOLATE'"`
		Columns     []InterpolateColumn `parser:"('(' (@@ (',' @@)*)? ')')?"`
	}

	// InterpolateColumn represents a column in INTERPOLATE clause
	InterpolateColumn struct {
		Name       string      `parser:"@(Ident | BacktickIdent)"`
		Expression *Expression `parser:"('AS' @@)?"`
	}

	// OrderByColumn represents a single column in ORDER BY
	OrderByColumn struct {
		Expression    Expression  `parser:"@@"`
		Direction     *string     `parser:"@('ASC' | 'DESC')?"`
		Nulls         *string     `parser:"('NULLS' @('FIRST' | 'LAST'))?"`
		Collate       *string     `parser:"('COLLATE' @String)?"`
		WithFill      bool        `parser:"@('WITH' 'FILL')?"`
		FillFrom      *Expression `parser:"('FROM' @@)?"`
		FillTo        *Expression `parser:"('TO' @@)?"`
		FillStep      *Expression `parser:"('STEP' @@)?"`
		FillStaleness *Expression `parser:"('STALENESS' @@)?"`
	}

	// LimitClause represents LIMIT clause
	LimitClause struct {
		Limit  string        `parser:"'LIMIT'"`
		Count  Expression    `parser:"@@"`
		Offset *OffsetClause `parser:"@@?"`
		By     *LimitBy      `parser:"@@?"`
	}

	// OffsetClause represents OFFSET clause
	OffsetClause struct {
		Offset string     `parser:"'OFFSET'"`
		Value  Expression `parser:"@@"`
	}

	// LimitBy represents LIMIT ... BY clause
	LimitBy struct {
		By      string       `parser:"'BY'"`
		Columns []Expression `parser:"@@ (',' @@)*"`
	}

	// SettingsClause represents SETTINGS clause
	SettingsClause struct {
		Settings string               `parser:"'SETTINGS'"`
		Values   []SettingsAssignment `parser:"@@ (',' @@)*"`
	}

	// SettingsAssignment represents key=value in SETTINGS
	SettingsAssignment struct {
		Key   string     `parser:"@(Ident | BacktickIdent)"`
		Value Expression `parser:"'=' @@"`
	}
)
