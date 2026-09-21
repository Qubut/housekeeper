package schema_test

import (
	"testing"

	"github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/pseudomuto/housekeeper/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestUnionLimitSelectStarDumpIsNoDiff(t *testing.T) {
	source := `CREATE VIEW events_batch ON CLUSTER '{cluster}' AS SELECT * FROM (SELECT id FROM events ORDER BY id DESC LIMIT (SELECT n FROM caps WHERE m = 'a')) UNION ALL SELECT * FROM (SELECT id FROM events ORDER BY id DESC LIMIT (SELECT n FROM caps WHERE m = 'b'));`
	dumped := `CREATE VIEW default.events_batch ON CLUSTER '{cluster}' AS SELECT * FROM (SELECT id FROM events ORDER BY id DESC LIMIT (SELECT n FROM caps WHERE m = 'a')) UNION ALL SELECT * FROM (SELECT id FROM events ORDER BY id DESC LIMIT (SELECT n FROM caps WHERE m = 'b'));`

	current, err := parser.ParseString(dumped)
	require.NoError(t, err)
	target, err := parser.ParseString(source)
	require.NoError(t, err)

	_, err = schema.GenerateDiff(current, target)
	require.ErrorIs(t, err, schema.ErrNoDiff)
}

func TestUnionLimitExpandedStarDumpIsNoDiff(t *testing.T) {
	source := `CREATE VIEW events_batch ON CLUSTER '{cluster}' AS SELECT * FROM (SELECT id, kind FROM events ORDER BY id DESC LIMIT (SELECT n FROM caps WHERE m = 'a')) UNION ALL SELECT * FROM (SELECT id, kind FROM events ORDER BY id DESC LIMIT (SELECT n FROM caps WHERE m = 'b'));`
	dumped := `CREATE VIEW default.events_batch ON CLUSTER '{cluster}' AS SELECT id, kind FROM (SELECT id, kind FROM events ORDER BY id DESC LIMIT (SELECT n FROM caps WHERE m = 'a')) UNION ALL SELECT id, kind FROM (SELECT id, kind FROM events ORDER BY id DESC LIMIT (SELECT n FROM caps WHERE m = 'b'));`

	current, err := parser.ParseString(dumped)
	require.NoError(t, err)
	target, err := parser.ParseString(source)
	require.NoError(t, err)

	_, err = schema.GenerateDiff(current, target)
	require.ErrorIs(t, err, schema.ErrNoDiff)
}

func TestRefreshableMVDumpIsNoDiff(t *testing.T) {
	source := `CREATE MATERIALIZED VIEW mv_hourly ON CLUSTER '{cluster}' REFRESH EVERY 1 HOUR APPEND TO hourly_snapshot AS SELECT * FROM (SELECT toStartOfHour(ts) AS hour, count() AS cnt FROM events GROUP BY hour ORDER BY hour DESC LIMIT (SELECT n FROM caps));`
	dumped := `CREATE MATERIALIZED VIEW default.mv_hourly ON CLUSTER '{cluster}' REFRESH EVERY 1 HOUR APPEND TO hourly_snapshot AS SELECT hour, cnt FROM (SELECT toStartOfHour(ts) AS hour, count() AS cnt FROM events GROUP BY hour ORDER BY hour DESC LIMIT (SELECT n FROM caps));`

	current, err := parser.ParseString(dumped)
	require.NoError(t, err)
	target, err := parser.ParseString(source)
	require.NoError(t, err)

	_, err = schema.GenerateDiff(current, target)
	require.ErrorIs(t, err, schema.ErrNoDiff)
}
