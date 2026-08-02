package schema_test

import (
	"testing"

	"github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/pseudomuto/housekeeper/pkg/schema"
	"github.com/stretchr/testify/require"
)

// ClickHouse's SHOW CREATE / system.tables always echoes an `IN <scalar>`
// condition back with its right-hand side wrapped in a single-element
// parenthesized list (`IN ({p:Array(UInt64)})`), even when the source DDL
// wrote the bare, unparenthesized form (`IN {p:Array(UInt64)}`) — the two
// spellings parse into different InExpression grammar alternatives (a
// one-element List vs a bare Expr) and denote the same value. Without
// folding both alternatives onto the same comparison key, any view whose
// source SQL writes a bare IN right-hand side (a query parameter, a column
// reference, or any other non-list scalar) compares as permanently different
// from its own live definition on every diff run.
func TestInBareScalarEquivalentToSingleElementList(t *testing.T) {
	current, err := parser.ParseString(
		`CREATE VIEW v ON CLUSTER '{cluster}' AS SELECT id FROM t WHERE id IN ({p:Array(UInt64)});`,
	)
	require.NoError(t, err)

	target, err := parser.ParseString(
		`CREATE VIEW v ON CLUSTER '{cluster}' AS SELECT id FROM t WHERE id IN {p:Array(UInt64)};`,
	)
	require.NoError(t, err)

	_, err = schema.GenerateDiff(current, target)
	require.ErrorIs(t, err, schema.ErrNoDiff, "IN ({p:Array(UInt64)}) and IN {p:Array(UInt64)} should compare equal")
}

// Same equivalence for a plain column reference on the right-hand side,
// confirming the fold isn't specific to the `{name:Type}` placeholder
// spelling.
func TestInBareColumnEquivalentToSingleElementList(t *testing.T) {
	current, err := parser.ParseString(
		`CREATE VIEW v ON CLUSTER '{cluster}' AS SELECT id FROM t WHERE id IN (other_id);`,
	)
	require.NoError(t, err)

	target, err := parser.ParseString(
		`CREATE VIEW v ON CLUSTER '{cluster}' AS SELECT id FROM t WHERE id IN other_id;`,
	)
	require.NoError(t, err)

	_, err = schema.GenerateDiff(current, target)
	require.ErrorIs(t, err, schema.ErrNoDiff, "IN (other_id) and IN other_id should compare equal")
}
