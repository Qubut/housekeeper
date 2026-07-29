package schema

import (
	"strings"

	"github.com/pseudomuto/housekeeper/pkg/parser"
)

// maxInlineDepth guards against runaway recursion from a mutually-referential
// function pair. ClickHouse itself rejects recursive SQL user-defined
// functions, so hitting this depth in practice would indicate a bug rather
// than a legitimate deep call chain; the fallback (stop inlining, keep the
// call as-is) degrades to a false-positive diff rather than a crash.
const maxInlineDepth = 16

// inlineFunctionCallsInSelect returns a structural copy of stmt with every
// call to a function in `functions` expanded to its body (formal parameters
// substituted with the call's actual arguments), recursing through
// subqueries, CTEs, and joins.
//
// This mirrors ClickHouse itself: `SHOW CREATE VIEW` / a materialized
// dictionary's `system.tables.create_table_query` never round-trips a call to
// a SQL user-defined function — ClickHouse permanently substitutes the
// expansion at CREATE time (verified empirically: a view calling
// `exp_half_life_weight(event_hour, 86400.)` comes back from
// `system.tables.create_table_query` as the fully expanded
// `exp(((-log(2)) * dateDiff(...)) / 86400.)`, with no trace of the function
// name). Comparing hand-written source — which still spells out the call —
// against that already-expanded live state can only ever match once target
// has been expanded the same way; no amount of AST-shape canonicalization on
// the comparator side can undo a substitution that already happened upstream.
//
// The rewrite is comparison-only: callers keep the original, un-inlined
// statement for generating migration SQL, so `CREATE OR REPLACE VIEW` text
// still reads `myFunc(x)`, not its expansion.
//
// Known gap: `CaseExpression` WHEN/THEN clauses are captured as raw token
// text by the grammar (parser.WhenClause), not as a nested Expression AST, so
// UDF calls inside CASE branches are not reachable here and will still
// perpetually re-diff if introduced. None of this project's views currently
// use CASE, so it is left as a documented limitation rather than a grammar
// change.
func inlineFunctionCallsInSelect(stmt *parser.SelectStatement, functions map[string]*FunctionInfo) *parser.SelectStatement {
	if stmt == nil || len(functions) == 0 {
		return stmt
	}
	return rewriteSelect(stmt, functions, 0)
}

func rewriteSelect(stmt *parser.SelectStatement, functions map[string]*FunctionInfo, depth int) *parser.SelectStatement {
	if stmt == nil {
		return nil
	}
	out := *stmt

	if stmt.With != nil {
		with := *stmt.With
		ctes := make([]parser.CommonTableExpression, len(stmt.With.CTEs))
		for i, cte := range stmt.With.CTEs {
			cte.Query = rewriteSelect(cte.Query, functions, depth)
			ctes[i] = cte
		}
		with.CTEs = ctes
		out.With = &with
	}

	cols := make([]parser.SelectColumn, len(stmt.Columns))
	for i, c := range stmt.Columns {
		if c.Expression != nil {
			c.Expression = rewriteExpr(c.Expression, nil, functions, depth)
		}
		cols[i] = c
	}
	out.Columns = cols

	out.From = rewriteFrom(stmt.From, functions, depth)

	if stmt.Where != nil {
		w := *stmt.Where
		w.Condition = *rewriteExpr(&stmt.Where.Condition, nil, functions, depth)
		out.Where = &w
	}

	if stmt.GroupBy != nil {
		gb := *stmt.GroupBy
		gCols := make([]parser.Expression, len(stmt.GroupBy.Columns))
		for i := range stmt.GroupBy.Columns {
			gCols[i] = *rewriteExpr(&stmt.GroupBy.Columns[i], nil, functions, depth)
		}
		gb.Columns = gCols
		out.GroupBy = &gb
	}

	if stmt.Having != nil {
		h := *stmt.Having
		h.Condition = *rewriteExpr(&stmt.Having.Condition, nil, functions, depth)
		out.Having = &h
	}

	if stmt.OrderBy != nil {
		ob := *stmt.OrderBy
		obCols := make([]parser.OrderByColumn, len(stmt.OrderBy.Columns))
		for i, c := range stmt.OrderBy.Columns {
			c.Expression = *rewriteExpr(&c.Expression, nil, functions, depth)
			obCols[i] = c
		}
		ob.Columns = obCols
		out.OrderBy = &ob
	}

	if stmt.Limit != nil {
		l := *stmt.Limit
		l.Count = *rewriteExpr(&stmt.Limit.Count, nil, functions, depth)
		if stmt.Limit.Offset != nil {
			off := *stmt.Limit.Offset
			off.Value = *rewriteExpr(&stmt.Limit.Offset.Value, nil, functions, depth)
			l.Offset = &off
		}
		out.Limit = &l
	}

	if len(stmt.Unions) > 0 {
		unions := make([]parser.UnionClause, len(stmt.Unions))
		for i := range stmt.Unions {
			unions[i] = rewriteUnion(stmt.Unions[i], functions, depth)
		}
		out.Unions = unions
	}

	return &out
}

// rewriteUnion expands UDF calls inside one UNION arm by reusing rewriteSelect
// on a temporary SelectStatement (no nested Unions / WITH on the arm).
func rewriteUnion(u parser.UnionClause, functions map[string]*FunctionInfo, depth int) parser.UnionClause {
	arm := &parser.SelectStatement{
		Select:   u.Select,
		Distinct: u.Distinct,
		Columns:  u.Columns,
		From:     u.From,
		Where:    u.Where,
		GroupBy:  u.GroupBy,
		Having:   u.Having,
		OrderBy:  u.OrderBy,
		Limit:    u.Limit,
		Settings: u.Settings,
	}
	rewritten := rewriteSelect(arm, functions, depth)
	u.Columns = rewritten.Columns
	u.From = rewritten.From
	u.Where = rewritten.Where
	u.GroupBy = rewritten.GroupBy
	u.Having = rewritten.Having
	u.OrderBy = rewritten.OrderBy
	u.Limit = rewritten.Limit
	u.Settings = rewritten.Settings
	return u
}

func rewriteFrom(from *parser.FromClause, functions map[string]*FunctionInfo, depth int) *parser.FromClause {
	if from == nil {
		return nil
	}
	out := *from
	out.Table = rewriteTableRef(from.Table, functions, depth)

	joins := make([]parser.JoinClause, len(from.Joins))
	for i, j := range from.Joins {
		j.Table = rewriteTableRef(j.Table, functions, depth)
		if j.Condition != nil && j.Condition.On != nil {
			cond := *j.Condition
			cond.On = rewriteExpr(j.Condition.On, nil, functions, depth)
			j.Condition = &cond
		}
		joins[i] = j
	}
	out.Joins = joins
	return &out
}

// rewriteTableRef recurses into FROM (subquery) targets. Table-function and
// plain table-name refs carry no UDF-call surface worth inlining.
func rewriteTableRef(ref parser.TableRef, functions map[string]*FunctionInfo, depth int) parser.TableRef {
	if ref.Subquery != nil {
		sub := *ref.Subquery
		rewritten := rewriteSelect(&ref.Subquery.Subquery, functions, depth)
		sub.Subquery = *rewritten
		ref.Subquery = &sub
	}
	return ref
}

// rewriteExpr walks the full expression grammar, substituting `subst`
// (formal parameter name, lowercased -> actual argument) at identifier
// leaves and expanding calls to `functions` (see inlineFunctionCallsInSelect)
// wherever they appear. `subst` is nil outside of an active function-body
// expansion.
func rewriteExpr(e *parser.Expression, subst map[string]*parser.Expression, functions map[string]*FunctionInfo, depth int) *parser.Expression {
	if e == nil {
		return nil
	}
	if e.Case != nil {
		return &parser.Expression{Case: e.Case}
	}
	return &parser.Expression{Or: rewriteOr(e.Or, subst, functions, depth)}
}

func rewriteOr(o *parser.OrExpression, subst map[string]*parser.Expression, functions map[string]*FunctionInfo, depth int) *parser.OrExpression {
	if o == nil {
		return nil
	}
	out := *o
	out.And = rewriteAnd(o.And, subst, functions, depth)
	rest := make([]parser.OrRest, len(o.Rest))
	for i, r := range o.Rest {
		r.And = rewriteAnd(r.And, subst, functions, depth)
		rest[i] = r
	}
	out.Rest = rest
	return &out
}

func rewriteAnd(a *parser.AndExpression, subst map[string]*parser.Expression, functions map[string]*FunctionInfo, depth int) *parser.AndExpression {
	if a == nil {
		return nil
	}
	out := *a
	out.Not = rewriteNot(a.Not, subst, functions, depth)
	rest := make([]parser.AndRest, len(a.Rest))
	for i, r := range a.Rest {
		r.Not = rewriteNot(r.Not, subst, functions, depth)
		rest[i] = r
	}
	out.Rest = rest
	return &out
}

func rewriteNot(n *parser.NotExpression, subst map[string]*parser.Expression, functions map[string]*FunctionInfo, depth int) *parser.NotExpression {
	if n == nil {
		return nil
	}
	out := *n
	out.Comparison = rewriteComparison(n.Comparison, subst, functions, depth)
	return &out
}

func rewriteComparison(c *parser.ComparisonExpression, subst map[string]*parser.Expression, functions map[string]*FunctionInfo, depth int) *parser.ComparisonExpression {
	if c == nil {
		return nil
	}
	out := *c
	out.Addition = rewriteAddition(c.Addition, subst, functions, depth)
	if c.Rest != nil && c.Rest.SimpleOp != nil {
		rest := *c.Rest
		op := *c.Rest.SimpleOp
		op.Addition = rewriteAddition(c.Rest.SimpleOp.Addition, subst, functions, depth)
		rest.SimpleOp = &op
		out.Rest = &rest
	}
	return &out
}

func rewriteAddition(a *parser.AdditionExpression, subst map[string]*parser.Expression, functions map[string]*FunctionInfo, depth int) *parser.AdditionExpression {
	if a == nil {
		return nil
	}
	out := *a
	out.Multiplication = rewriteMultiplication(a.Multiplication, subst, functions, depth)
	rest := make([]parser.AdditionRest, len(a.Rest))
	for i, r := range a.Rest {
		r.Multiplication = rewriteMultiplication(r.Multiplication, subst, functions, depth)
		rest[i] = r
	}
	out.Rest = rest
	return &out
}

func rewriteMultiplication(m *parser.MultiplicationExpression, subst map[string]*parser.Expression, functions map[string]*FunctionInfo, depth int) *parser.MultiplicationExpression {
	if m == nil {
		return nil
	}
	out := *m
	out.Unary = rewriteUnary(m.Unary, subst, functions, depth)
	rest := make([]parser.MultiplicationRest, len(m.Rest))
	for i, r := range m.Rest {
		r.Unary = rewriteUnary(r.Unary, subst, functions, depth)
		rest[i] = r
	}
	out.Rest = rest
	return &out
}

func rewriteUnary(u *parser.UnaryExpression, subst map[string]*parser.Expression, functions map[string]*FunctionInfo, depth int) *parser.UnaryExpression {
	if u == nil {
		return nil
	}
	out := *u
	out.Primary = rewritePrimary(u.Primary, subst, functions, depth)
	return &out
}

func rewritePrimary(p *parser.PrimaryExpression, subst map[string]*parser.Expression, functions map[string]*FunctionInfo, depth int) *parser.PrimaryExpression {
	if p == nil {
		return nil
	}
	switch {
	case p.Identifier != nil && p.Identifier.Database == nil && p.Identifier.Table == nil:
		if repl, ok := subst[strings.ToLower(p.Identifier.Name)]; ok && repl != nil {
			return &parser.PrimaryExpression{Parentheses: &parser.ParenExpression{Expression: *repl}}
		}
		return p
	case p.Function != nil:
		return rewriteFunctionCall(p.Function, subst, functions, depth)
	case p.Parentheses != nil:
		inner := rewriteExpr(&p.Parentheses.Expression, subst, functions, depth)
		return &parser.PrimaryExpression{Parentheses: &parser.ParenExpression{Expression: *inner}}
	case p.Tuple != nil:
		elems := make([]parser.Expression, len(p.Tuple.Elements))
		for i := range p.Tuple.Elements {
			elems[i] = *rewriteExpr(&p.Tuple.Elements[i], subst, functions, depth)
		}
		return &parser.PrimaryExpression{Tuple: &parser.TupleExpression{Elements: elems}}
	case p.Array != nil:
		elems := make([]parser.Expression, len(p.Array.Elements))
		for i := range p.Array.Elements {
			elems[i] = *rewriteExpr(&p.Array.Elements[i], subst, functions, depth)
		}
		return &parser.PrimaryExpression{Array: &parser.ArrayExpression{Elements: elems}}
	default:
		// Literal / Cast / Interval / Extract carry no identifiers or calls
		// relevant to our UDF bodies; left as-is.
		return p
	}
}

// rewriteFunctionCall substitutes `subst` into the call's own arguments
// first (so a UDF parameter reference used as another call's argument is
// resolved before that outer call is potentially inlined too), then, if the
// call targets a known function with a matching arity and no second
// (parameterized) argument list, splices the callee's body in place of the
// call — wrapped in a synthetic ParenExpression so it can occupy the
// PrimaryExpression slot the call used to occupy. canonicalExprKey already
// dissolves such grouping parens, so the inlined body compares cleanly
// against ClickHouse's own (paren-heavy) expansion.
func rewriteFunctionCall(f *parser.FunctionCall, subst map[string]*parser.Expression, functions map[string]*FunctionInfo, depth int) *parser.PrimaryExpression {
	args := make([]parser.FunctionArg, len(f.FirstParentheses))
	for i, a := range f.FirstParentheses {
		if a.Expression != nil {
			a.Expression = rewriteExpr(a.Expression, subst, functions, depth)
		}
		args[i] = a
	}

	name := strings.ToLower(f.Name)
	if fn, ok := functions[name]; ok && depth < maxInlineDepth &&
		len(f.SecondParentheses) == 0 && len(fn.Parameters) == len(args) {
		inner := make(map[string]*parser.Expression, len(fn.Parameters))
		for i, param := range fn.Parameters {
			if args[i].Expression != nil {
				inner[strings.ToLower(param)] = args[i].Expression
			}
		}
		body := rewriteExpr(fn.Expression, inner, functions, depth+1)
		return &parser.PrimaryExpression{Parentheses: &parser.ParenExpression{Expression: *body}}
	}

	newFn := *f
	newFn.FirstParentheses = args
	return &parser.PrimaryExpression{Function: &newFn}
}

// lowercaseFunctionKeys re-keys a function map by lowercased name for
// case-insensitive lookup during inlining (ClickHouse function names are
// case-insensitive; extractFunctionInfoAsMap keys by as-declared spelling).
func lowercaseFunctionKeys(functions map[string]*FunctionInfo) map[string]*FunctionInfo {
	out := make(map[string]*FunctionInfo, len(functions))
	for name, fn := range functions {
		out[strings.ToLower(name)] = fn
	}
	return out
}
