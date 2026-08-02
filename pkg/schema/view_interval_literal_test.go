package schema_test

import (
	"testing"

	"github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/pseudomuto/housekeeper/pkg/schema"
	"github.com/stretchr/testify/require"
)

// ClickHouse always echoes date-arithmetic literals back in their
// function-call spelling (`toIntervalDay(8)`) in SHOW CREATE / system.tables,
// regardless of whether the source DDL used that spelling or the `INTERVAL n
// UNIT` literal syntax. Without folding both spellings onto the same
// comparison key, a view whose source SQL uses `INTERVAL n UNIT` compares as
// permanently different from its own live definition on every diff run.
func TestIntervalLiteralEquivalentToToIntervalFunction(t *testing.T) {
	current, err := parser.ParseString(
		`CREATE VIEW v ON CLUSTER '{cluster}' AS SELECT id FROM t WHERE ts >= now() - toIntervalDay(8);`,
	)
	require.NoError(t, err)

	target, err := parser.ParseString(
		`CREATE VIEW v ON CLUSTER '{cluster}' AS SELECT id FROM t WHERE ts >= now() - INTERVAL 8 DAY;`,
	)
	require.NoError(t, err)

	_, err = schema.GenerateDiff(current, target)
	require.ErrorIs(t, err, schema.ErrNoDiff, "INTERVAL 8 DAY and toIntervalDay(8) should compare equal")
}
