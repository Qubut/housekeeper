package schema_test

import (
	"testing"

	"github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/pseudomuto/housekeeper/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestInSubqueryIdenticalIsNoDiff(t *testing.T) {
	ddl := `CREATE VIEW events_batch ON CLUSTER '{cluster}' AS SELECT id FROM events WHERE kind IN (SELECT kind FROM event_kinds);`

	current, err := parser.ParseString(ddl)
	require.NoError(t, err)
	target, err := parser.ParseString(ddl)
	require.NoError(t, err)

	_, err = schema.GenerateDiff(current, target)
	require.ErrorIs(t, err, schema.ErrNoDiff)
}

func TestAddingInSubqueryProducesParseableDiff(t *testing.T) {
	current, err := parser.ParseString(
		`CREATE VIEW events_batch ON CLUSTER '{cluster}' AS SELECT id FROM events;`,
	)
	require.NoError(t, err)

	target, err := parser.ParseString(
		`CREATE VIEW events_batch ON CLUSTER '{cluster}' AS SELECT id FROM events WHERE kind IN (SELECT kind FROM event_kinds);`,
	)
	require.NoError(t, err)

	diff, err := schema.GenerateDiff(current, target)
	require.NoError(t, err)
	require.NotNil(t, diff)
	require.NotEmpty(t, diff.Statements)
}
