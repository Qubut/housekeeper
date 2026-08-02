package parser_test

import (
	"bytes"
	"testing"

	"github.com/pseudomuto/housekeeper/pkg/format"
	. "github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/stretchr/testify/require"
)

// TestParameterExprParsing verifies the ClickHouse query-parameter placeholder
// ({name:Type}) parses into a ParameterExpr at every expression position a
// parameterized view needs: WHERE comparisons, IN clauses, and function args.
func TestParameterExprParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sql  string
	}{
		{
			name: "simple type in WHERE equality",
			sql:  `CREATE VIEW v_by_id AS SELECT id FROM items WHERE id = {id:UInt64};`,
		},
		{
			name: "array type in IN clause",
			sql:  `CREATE VIEW v_by_ids AS SELECT id FROM items WHERE id IN {ids:Array(UInt64)};`,
		},
		{
			name: "array type as function argument",
			sql:  `CREATE VIEW v_spine AS SELECT arrayJoin({ids:Array(UInt64)}) AS id;`,
		},
		{
			name: "nullable datetime type",
			sql:  `CREATE VIEW v_cutoff AS SELECT id FROM items WHERE updated_at > {cutoff:Nullable(DateTime64(6))};`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sqlResult, err := ParseString(tt.sql)
			require.NoError(t, err)
			require.Len(t, sqlResult.Statements, 1)
			require.NotNil(t, sqlResult.Statements[0].CreateView)
		})
	}
}

// TestParameterExprRoundTripIdempotent confirms CREATE VIEW ... {name:Type} ...
// round-trips through ParseString -> Format -> ParseString -> Format without
// drift, matching housekeeper's own idempotency guarantee (a clean re-run of
// ch-migrate-diff against already-applied schema must produce no diff).
func TestParameterExprRoundTripIdempotent(t *testing.T) {
	t.Parallel()

	sql := `CREATE VIEW v_buff163_poll_candidates AS ` +
		`SELECT c.item_id AS item_id, w.last_polled AS last_polled ` +
		`FROM (SELECT arrayJoin({ids:Array(UInt64)}) AS item_id) AS c ` +
		`LEFT JOIN (SELECT item_id, max(last_updated) AS last_polled FROM update_history ` +
		`WHERE source = 'buff163' AND item_id IN {ids:Array(UInt64)} GROUP BY item_id) AS w ` +
		`ON w.item_id = c.item_id;`

	firstParse, err := ParseString(sql)
	require.NoError(t, err)

	var firstBuf bytes.Buffer
	require.NoError(t, format.Format(&firstBuf, format.Defaults, firstParse.Statements...))
	firstFormatted := firstBuf.String()
	require.Contains(t, firstFormatted, "{ids:Array(UInt64)}",
		"formatted output must preserve the parameter placeholder verbatim, not drop it")

	secondParse, err := ParseString(firstFormatted)
	require.NoError(t, err, "re-parsing the formatter's own output must succeed")

	var secondBuf bytes.Buffer
	require.NoError(t, format.Format(&secondBuf, format.Defaults, secondParse.Statements...))
	secondFormatted := secondBuf.String()

	require.Equal(t, firstFormatted, secondFormatted,
		"parse -> format -> parse -> format must be a fixed point (idempotent)")
}

// TestParameterExprEqual verifies ParameterExpr participates correctly in the
// PrimaryExpression.Equal structural comparison used by housekeeper's schema
// diffing, so identical parameterized views don't produce spurious migrations
// and differing ones (name or type) are correctly detected as changed.
func TestParameterExprEqual(t *testing.T) {
	t.Parallel()

	parseExpr := func(t *testing.T, sql string) *PrimaryExpression {
		t.Helper()
		result, err := ParseString(`CREATE VIEW v AS SELECT id FROM items WHERE id = ` + sql + `;`)
		require.NoError(t, err)
		view := result.Statements[0].CreateView
		require.NotNil(t, view)
		where := view.AsSelect.Where
		require.NotNil(t, where)
		// Navigate to the right-hand side of "id = {...}" — the parameter itself.
		rhs := where.Condition.Or.And.Not.Comparison.Rest.SimpleOp.Addition
		return rhs.Multiplication.Unary.Primary
	}

	a := parseExpr(t, "{id:UInt64}")
	bSame := parseExpr(t, "{id:UInt64}")
	cDifferentName := parseExpr(t, "{other_id:UInt64}")
	dDifferentType := parseExpr(t, "{id:Int64}")

	require.True(t, a.Equal(bSame))
	require.False(t, a.Equal(cDifferentName))
	require.False(t, a.Equal(dDifferentType))
}
