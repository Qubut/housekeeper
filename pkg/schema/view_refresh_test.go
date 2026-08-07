package schema_test

import (
	"strings"
	"testing"

	"github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/pseudomuto/housekeeper/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestRefreshableMVIdenticalIsNoDiff(t *testing.T) {
	ddl := `CREATE MATERIALIZED VIEW mv ON CLUSTER '{cluster}' REFRESH EVERY 30 SECOND APPEND TO target_table AS SELECT 1 AS x;`

	current, err := parser.ParseString(ddl)
	require.NoError(t, err)
	target, err := parser.ParseString(ddl)
	require.NoError(t, err)

	_, err = schema.GenerateDiff(current, target)
	require.ErrorIs(t, err, schema.ErrNoDiff)
}

func TestRefreshUnitPluralEquivalentToSingular(t *testing.T) {
	current, err := parser.ParseString(
		`CREATE MATERIALIZED VIEW mv ON CLUSTER '{cluster}' REFRESH EVERY 30 SECOND APPEND TO target_table AS SELECT 1 AS x;`,
	)
	require.NoError(t, err)

	target, err := parser.ParseString(
		`CREATE MATERIALIZED VIEW mv ON CLUSTER '{cluster}' REFRESH EVERY 30 SECONDS APPEND TO target_table AS SELECT 1 AS x;`,
	)
	require.NoError(t, err)

	_, err = schema.GenerateDiff(current, target)
	require.ErrorIs(t, err, schema.ErrNoDiff, "SECOND and SECONDS should compare equal")
}

func TestRefreshAppendChangeProducesDropCreate(t *testing.T) {
	current, err := parser.ParseString(
		`CREATE MATERIALIZED VIEW mv ON CLUSTER '{cluster}' REFRESH EVERY 30 SECOND TO target_table AS SELECT 1 AS x;`,
	)
	require.NoError(t, err)

	target, err := parser.ParseString(
		`CREATE MATERIALIZED VIEW mv ON CLUSTER '{cluster}' REFRESH EVERY 30 SECOND APPEND TO target_table AS SELECT 1 AS x;`,
	)
	require.NoError(t, err)

	diff, err := schema.GenerateDiff(current, target)
	require.NoError(t, err)
	require.NotNil(t, diff)

	var buf strings.Builder
	for _, stmt := range diff.Statements {
		if stmt.DropTable != nil {
			buf.WriteString("DROP;")
		}
		if stmt.CreateView != nil {
			require.NotNil(t, stmt.CreateView.Refresh)
			require.True(t, stmt.CreateView.Refresh.Append)
			buf.WriteString("CREATE;")
		}
	}
	require.Contains(t, buf.String(), "DROP;")
	require.Contains(t, buf.String(), "CREATE;")
}

func TestRefreshIntervalChangeProducesDiff(t *testing.T) {
	current, err := parser.ParseString(
		`CREATE MATERIALIZED VIEW mv ON CLUSTER '{cluster}' REFRESH EVERY 30 SECOND APPEND TO target_table AS SELECT 1 AS x;`,
	)
	require.NoError(t, err)

	target, err := parser.ParseString(
		`CREATE MATERIALIZED VIEW mv ON CLUSTER '{cluster}' REFRESH EVERY 1 MINUTE APPEND TO target_table AS SELECT 1 AS x;`,
	)
	require.NoError(t, err)

	diff, err := schema.GenerateDiff(current, target)
	require.NoError(t, err)
	require.NotNil(t, diff)

	found := false
	for _, stmt := range diff.Statements {
		if stmt.CreateView != nil && stmt.CreateView.Refresh != nil {
			require.Equal(t, "EVERY", stmt.CreateView.Refresh.Kind)
			require.NotNil(t, stmt.CreateView.Refresh.Period)
			require.Equal(t, "1", stmt.CreateView.Refresh.Period.Parts[0].Value)
			require.Equal(t, "MINUTE", stmt.CreateView.Refresh.Period.Parts[0].Unit)
			found = true
		}
	}
	require.True(t, found, "expected CREATE with updated REFRESH EVERY 1 MINUTE")
}

func TestRefreshCreateMigrationIncludesRefreshClause(t *testing.T) {
	current, err := parser.ParseString(``)
	require.NoError(t, err)

	target, err := parser.ParseString(
		`CREATE MATERIALIZED VIEW db.mv_refresh ON CLUSTER '{cluster}' REFRESH EVERY 30 SECOND APPEND TO db.target_table AS SELECT 1 AS x;`,
	)
	require.NoError(t, err)

	diff, err := schema.GenerateDiff(current, target)
	require.NoError(t, err)
	require.NotNil(t, diff)
	require.Len(t, diff.Statements, 1)
	require.NotNil(t, diff.Statements[0].CreateView)
	require.NotNil(t, diff.Statements[0].CreateView.Refresh)
	require.True(t, diff.Statements[0].CreateView.Refresh.Append)
	require.Equal(t, "EVERY", diff.Statements[0].CreateView.Refresh.Kind)
}
