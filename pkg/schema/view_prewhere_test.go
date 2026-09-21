package schema_test

import (
	"testing"

	"github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/pseudomuto/housekeeper/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestPrewhereIdenticalIsNoDiff(t *testing.T) {
	ddl := `CREATE VIEW events_recent ON CLUSTER '{cluster}' AS SELECT id FROM events PREWHERE kind = 'sale' WHERE ts > now();`

	current, err := parser.ParseString(ddl)
	require.NoError(t, err)
	target, err := parser.ParseString(ddl)
	require.NoError(t, err)

	_, err = schema.GenerateDiff(current, target)
	require.ErrorIs(t, err, schema.ErrNoDiff)
}

func TestPrewhereConditionChangeProducesDiff(t *testing.T) {
	current, err := parser.ParseString(
		`CREATE VIEW events_recent ON CLUSTER '{cluster}' AS SELECT id FROM events PREWHERE kind = 'sale' WHERE ts > now();`,
	)
	require.NoError(t, err)

	target, err := parser.ParseString(
		`CREATE VIEW events_recent ON CLUSTER '{cluster}' AS SELECT id FROM events PREWHERE kind = 'listing' WHERE ts > now();`,
	)
	require.NoError(t, err)

	diff, err := schema.GenerateDiff(current, target)
	require.NoError(t, err)
	require.NotNil(t, diff)
	require.NotEmpty(t, diff.Statements)
}

func TestAddingPrewhereProducesDiff(t *testing.T) {
	current, err := parser.ParseString(
		`CREATE VIEW events_recent ON CLUSTER '{cluster}' AS SELECT id FROM events WHERE ts > now();`,
	)
	require.NoError(t, err)

	target, err := parser.ParseString(
		`CREATE VIEW events_recent ON CLUSTER '{cluster}' AS SELECT id FROM events PREWHERE kind = 'sale' WHERE ts > now();`,
	)
	require.NoError(t, err)

	diff, err := schema.GenerateDiff(current, target)
	require.NoError(t, err)
	require.NotNil(t, diff)
	require.NotEmpty(t, diff.Statements)
}
