package schema

import (
	"iter"
	"strings"

	A "github.com/IBM/fp-go/v2/array"
	O "github.com/IBM/fp-go/v2/option"
	"github.com/pseudomuto/housekeeper/pkg/compare"
	"github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/pseudomuto/housekeeper/pkg/utils"
)

// inferSchemaClusterFromStrings picks the authoritative ON CLUSTER name:
// any server macro wins; otherwise a unanimous plain name; otherwise "".
func inferSchemaClusterFromStrings(clusters []string) string {
	return O.MonadGetOrElse(
		O.MonadAlt(
			A.FindFirst(utils.IsClickHouseMacro)(clusters),
			func() O.Option[string] {
				return O.MonadChain(A.Head(clusters), func(first string) O.Option[string] {
					if A.Any(func(c string) bool { return c != first })(clusters) {
						return O.None[string]()
					}
					return O.Some(first)
				})
			},
		),
		func() string { return "" },
	)
}

// ReconcileClusters rewrites every non-empty ON CLUSTER value in `current` to the
// name inferred from `target` (see inferSchemaClusterFromStrings), so live-DB
// state extracted with macros expanded compares equal to source schema written
// with the macros intact. No-op when target has no consistent cluster.
func ReconcileClusters[T any](current, target iter.Seq[T], get func(T) string, set func(T, string)) {
	var clusters []string
	for item := range target {
		if c := get(item); c != "" {
			clusters = append(clusters, c)
		}
	}
	sc := inferSchemaClusterFromStrings(clusters)
	if sc == "" {
		return
	}
	for item := range current {
		if get(item) != "" {
			set(item, sc)
		}
	}
}

// normalizeDefaultDatabase strips ClickHouse's implicit "default" database name
// so qualified ("default.foo") and unqualified ("foo") references share one key.
func normalizeDefaultDatabase(db string) string {
	if normalizeIdentifier(db) == "default" {
		return ""
	}
	return normalizeIdentifier(db)
}

// removeQuotes removes surrounding single quotes from a string
func removeQuotes(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}

	return s
}

// getStringValue safely gets a string value from a string pointer
func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// getViewTableTargetValue converts ViewTableTarget to string representation
func getViewTableTargetValue(target *parser.ViewTableTarget) string {
	if target == nil {
		return ""
	}

	if target.Function != nil {
		// Format table function as string
		result := target.Function.Name + "("
		var args []string
		for _, arg := range target.Function.Arguments {
			if arg.Star != nil {
				args = append(args, "*")
			} else if arg.Expression != nil {
				args = append(args, arg.Expression.String())
			}
		}
		result += strings.Join(args, ", ") + ")"
		return result
	} else if target.Table != nil {
		if target.Database != nil && normalizeDefaultDatabase(*target.Database) != "" {
			return *target.Database + "." + *target.Table
		}
		return *target.Table
	}

	return ""
}

// normalizeIdentifier removes surrounding backticks from ClickHouse identifiers
// for consistent comparison between parsed DDL and ClickHouse system table output
func normalizeIdentifier(s string) string {
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
		return s[1 : len(s)-1]
	}
	return s
}

// AST-based expression comparison, via canonical normalization
//
// ClickHouse's own `create_query` output (system.functions, SHOW CREATE VIEW,
// ...) does not round-trip a function/view body byte-for-byte: it re-serializes
// through its own internal AST, which (a) fully parenthesizes every binary
// operation regardless of whether the grouping was already implied by
// precedence/associativity, and (b) freely rewrites sugar such as
// `isNull(x)`/`isNotNull(x)` into the equivalent `x IS NULL`/`x IS NOT NULL`
// postfix form (and likely other operator/function rewrites this project
// hasn't hit yet). A naive structural (parenthesis- and spelling-sensitive)
// AST comparison therefore reports a permanent, spurious difference between
// hand-written source DDL and the live schema it created — every `diff` run
// re-emits CREATE FUNCTION/CREATE OR REPLACE VIEW for objects that never
// actually changed (see .cursor/adr for the housekeeper-function-diff-loop
// investigation).
//
// canonicalExprKey renders a parenthesization- and precedence-invariant key by
// walking the fully precedence-resolved AST and re-emitting each operator in a
// fixed prefix (S-expression) form; grouping parentheses are dissolved because
// they carry no information once precedence has already produced the tree
// shape, and `isNull`/`isNotNull` fold into the same key as `IS [NOT] NULL` so
// either spelling compares equal.
func expressionsAreEqual(expr1, expr2 *parser.Expression) bool {
	if eq, needsMoreChecks := compare.NilCheck(expr1, expr2); !needsMoreChecks {
		return eq
	}
	return canonicalExprKey(expr1) == canonicalExprKey(expr2)
}

func canonicalExprKey(e *parser.Expression) string {
	if e == nil {
		return ""
	}
	if e.Case != nil {
		return canonicalCaseKey(e.Case)
	}
	return canonicalOrKey(e.Or)
}

func canonicalCaseKey(c *parser.CaseExpression) string {
	if c == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("(CASE")
	for _, when := range c.WhenClauses {
		b.WriteString(" (WHEN ")
		b.WriteString(strings.TrimSpace(when.Condition))
		b.WriteString(" THEN ")
		b.WriteString(strings.TrimSpace(when.Result))
		b.WriteString(")")
	}
	if c.ElseClause != nil {
		b.WriteString(" (ELSE ")
		b.WriteString(strings.TrimSpace(c.ElseClause.Result))
		b.WriteString(")")
	}
	b.WriteString(")")
	return b.String()
}

func canonicalOrKey(o *parser.OrExpression) string {
	if o == nil {
		return ""
	}
	key := canonicalAndKey(o.And)
	for _, rest := range o.Rest {
		key = "(OR " + key + " " + canonicalAndKey(rest.And) + ")"
	}
	return key
}

func canonicalAndKey(a *parser.AndExpression) string {
	if a == nil {
		return ""
	}
	key := canonicalNotKey(a.Not)
	for _, rest := range a.Rest {
		key = "(AND " + key + " " + canonicalNotKey(rest.Not) + ")"
	}
	return key
}

func canonicalNotKey(n *parser.NotExpression) string {
	if n == nil {
		return ""
	}
	key := canonicalComparisonKey(n.Comparison)
	if n.Not {
		key = "(NOT " + key + ")"
	}
	return key
}

// canonicalComparisonKey folds the `IS [NOT] NULL` postfix onto the same
// `(FUNC isnull ...)` / `(FUNC isnotnull ...)` shape produced for the
// `isNull()`/`isNotNull()` call spelling in canonicalPrimaryKey, since
// ClickHouse treats the two as interchangeable.
func canonicalComparisonKey(c *parser.ComparisonExpression) string {
	if c == nil {
		return ""
	}
	key := canonicalAdditionKey(c.Addition)
	if c.Rest != nil {
		switch {
		case c.Rest.SimpleOp != nil:
			key = "(CMP " + simpleComparisonOpKey(c.Rest.SimpleOp.Op) + " " + key + " " + canonicalAdditionKey(c.Rest.SimpleOp.Addition) + ")"
		case c.Rest.InOp != nil:
			op := "IN"
			if c.Rest.InOp.Not {
				op = "NOT_IN"
			}
			key = "(CMP " + op + " " + key + " " + canonicalInExprKey(c.Rest.InOp.Expr) + ")"
		case c.Rest.BetweenOp != nil:
			op := "BETWEEN"
			if c.Rest.BetweenOp.Not {
				op = "NOT_BETWEEN"
			}
			key = "(CMP " + op + " " + key + " " + c.Rest.BetweenOp.Expr.String() + ")"
		}
	}
	if c.IsNull != nil {
		fn := "isnull"
		if c.IsNull.Not {
			fn = "isnotnull"
		}
		key = "(FUNC " + fn + " " + key + ")"
	}
	return key
}

func simpleComparisonOpKey(op *parser.SimpleComparisonOp) string {
	return op.String()
}

// canonicalInExprKey folds IN's right-hand side onto the same key regardless
// of whether the source wrote a bare scalar/placeholder/column or ClickHouse
// echoed it back wrapped in a single-element parenthesized list (`IN
// {p:Array(UInt64)}` vs `IN ({p:Array(UInt64)})`) — both denote the same
// value, and ClickHouse's SHOW CREATE always normalizes a bare RHS into the
// parenthesized-list form, so without this fold any `IN <placeholder>` or
// `IN <column>` condition compares as permanently different from its own
// live definition.
func canonicalInExprKey(in *parser.InExpression) string {
	switch {
	case in == nil:
		return ""
	case in.Expr != nil:
		return canonicalExprKey(in.Expr)
	case len(in.List) == 1:
		return canonicalExprKey(&in.List[0])
	case len(in.List) > 1:
		return "(LIST" + canonicalExprListSuffix(in.List) + ")"
	case in.Array != nil:
		return "(ARRAY" + canonicalExprListSuffix(in.Array.Elements) + ")"
	case in.Subquery != nil:
		return "(SUBQUERY " + in.Subquery.String() + ")"
	default:
		return ""
	}
}

func canonicalAdditionKey(a *parser.AdditionExpression) string {
	if a == nil {
		return ""
	}
	key := canonicalMultiplicationKey(a.Multiplication)
	for _, rest := range a.Rest {
		key = "(" + rest.Op + " " + key + " " + canonicalMultiplicationKey(rest.Multiplication) + ")"
	}
	return key
}

func canonicalMultiplicationKey(m *parser.MultiplicationExpression) string {
	if m == nil {
		return ""
	}
	key := canonicalUnaryKey(m.Unary)
	for _, rest := range m.Rest {
		key = "(" + rest.Op + " " + key + " " + canonicalUnaryKey(rest.Unary) + ")"
	}
	return key
}

func canonicalUnaryKey(u *parser.UnaryExpression) string {
	if u == nil {
		return ""
	}
	key := canonicalPrimaryKey(u.Primary)
	if u.Op != "" {
		key = "(UNARY " + u.Op + " " + key + ")"
	}
	return key
}

// canonicalPrimaryKey dissolves ParenExpression grouping (recursing back into
// canonicalExprKey) and folds `isNull`/`isNotNull` function-call spelling onto
// the same key as the `IS [NOT] NULL` postfix form (canonicalComparisonKey).
func canonicalPrimaryKey(p *parser.PrimaryExpression) string {
	if p == nil {
		return ""
	}
	switch {
	case p.Literal != nil:
		return canonicalLiteralKey(p.Literal)
	case p.Identifier != nil:
		return canonicalIdentifierKey(p.Identifier)
	case p.Function != nil:
		return canonicalFunctionKey(p.Function)
	case p.Parentheses != nil:
		return canonicalExprKey(&p.Parentheses.Expression)
	case p.Tuple != nil:
		return "(TUPLE" + canonicalExprListSuffix(p.Tuple.Elements) + ")"
	case p.Array != nil:
		return "(ARRAY" + canonicalExprListSuffix(p.Array.Elements) + ")"
	case p.Cast != nil:
		return "(CAST " + p.Cast.String() + ")"
	case p.Interval != nil:
		return canonicalIntervalKey(p.Interval)
	case p.Extract != nil:
		return "(EXTRACT " + p.Extract.String() + ")"
	default:
		return ""
	}
}

// canonicalIntervalKey folds ClickHouse's `INTERVAL n UNIT` literal syntax
// onto the same key as the `toInterval<Unit>(n)` function-call form: ClickHouse
// always echoes date arithmetic back in the function-call spelling in
// SHOW CREATE, regardless of which spelling the source DDL used, so without
// this fold every view whose source SQL writes `INTERVAL n UNIT` compares as
// permanently different from its own live definition.
func canonicalIntervalKey(iv *parser.IntervalExpr) string {
	return "(FUNC tointerval" + strings.ToLower(iv.Unit) + " (NUM " + iv.Value + "))"
}

func canonicalExprListSuffix(exprs []parser.Expression) string {
	var b strings.Builder
	for _, e := range exprs {
		b.WriteString(" ")
		b.WriteString(canonicalExprKey(&e))
	}
	return b.String()
}

func canonicalLiteralKey(l *parser.Literal) string {
	switch {
	case l.StringValue != nil:
		return "(STR " + *l.StringValue + ")"
	case l.Number != nil:
		return "(NUM " + *l.Number + ")"
	case l.Boolean != nil:
		return "(BOOL " + strings.ToUpper(*l.Boolean) + ")"
	case l.Null:
		return "(NULL)"
	default:
		return "()"
	}
}

// canonicalIdentifierKey lowercases the column name: ClickHouse identifiers
// compare case-insensitively for our purposes (matches prior comparator
// behavior for identifiersEqual/functionCallsEqual).
func canonicalIdentifierKey(id *parser.IdentifierExpr) string {
	var b strings.Builder
	b.WriteString("(ID")
	if id.Database != nil {
		b.WriteString(" " + normalizeIdentifier(*id.Database))
	}
	if id.Table != nil {
		b.WriteString(" " + normalizeIdentifier(*id.Table))
	}
	b.WriteString(" " + strings.ToLower(normalizeIdentifier(id.Name)))
	b.WriteString(")")
	return b.String()
}

func canonicalFunctionKey(f *parser.FunctionCall) string {
	name := strings.ToLower(f.Name)
	if (name == "isnull" || name == "isnotnull") && len(f.FirstParentheses) == 1 && len(f.SecondParentheses) == 0 {
		return "(FUNC " + name + " " + canonicalFunctionArgKey(&f.FirstParentheses[0]) + ")"
	}

	var b strings.Builder
	b.WriteString("(FUNC " + name)
	for _, arg := range f.FirstParentheses {
		b.WriteString(" ")
		b.WriteString(canonicalFunctionArgKey(&arg))
	}
	if len(f.SecondParentheses) > 0 {
		b.WriteString(" |")
		for _, arg := range f.SecondParentheses {
			b.WriteString(" ")
			b.WriteString(canonicalFunctionArgKey(&arg))
		}
	}
	if f.Over != nil {
		b.WriteString(" ")
		b.WriteString(f.Over.String())
	}
	b.WriteString(")")
	return b.String()
}

func canonicalFunctionArgKey(arg *parser.FunctionArg) string {
	if arg.Star != nil {
		return "*"
	}
	if arg.Expression != nil {
		return canonicalExprKey(arg.Expression)
	}
	return ""
}

// engineParametersEqual compares engine parameters
func engineParametersEqual(param1, param2 *parser.EngineParameter) bool {
	if param1 == nil && param2 == nil {
		return true
	}
	if param1 == nil || param2 == nil {
		return false
	}

	// Compare expressions
	if param1.Expression != nil && param2.Expression != nil {
		return expressionsAreEqual(param1.Expression, param2.Expression)
	}
	if param1.Expression != nil || param2.Expression != nil {
		return false
	}

	// Compare string values
	if param1.String != nil && param2.String != nil {
		return *param1.String == *param2.String
	}
	if param1.String != nil || param2.String != nil {
		return false
	}

	// Compare number values
	if param1.Number != nil && param2.Number != nil {
		return *param1.Number == *param2.Number
	}
	if param1.Number != nil || param2.Number != nil {
		return false
	}

	// Compare identifier values
	if param1.Ident != nil && param2.Ident != nil {
		return strings.EqualFold(*param1.Ident, *param2.Ident)
	}
	if param1.Ident != nil || param2.Ident != nil {
		return false
	}

	return true
}

// functionArgsEqual compares function arguments (can be * or expressions)
func functionArgsEqual(arg1, arg2 *parser.FunctionArg) bool {
	if arg1 == nil && arg2 == nil {
		return true
	}
	if arg1 == nil || arg2 == nil {
		return false
	}

	// Compare star arguments
	if arg1.Star != nil && arg2.Star != nil {
		return *arg1.Star == *arg2.Star
	}
	if arg1.Star != nil || arg2.Star != nil {
		return false
	}

	// Compare expression arguments
	if arg1.Expression != nil && arg2.Expression != nil {
		return expressionsAreEqual(arg1.Expression, arg2.Expression)
	}
	if arg1.Expression != nil || arg2.Expression != nil {
		return false
	}

	return true
}
