package clickhouse

import (
	"testing"

	"github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/stretchr/testify/require"
)

func TestStripDumpedViewHeaderKeepsRefreshAndTo(t *testing.T) {
	dumped := `CREATE MATERIALIZED VIEW default.mv_hourly
(
    ` + "`hour`" + ` DateTime,
    ` + "`cnt`" + ` UInt64
)
DEFINER = default SQL SECURITY DEFINER
REFRESH EVERY 1 HOUR TO default.hourly_snapshot
AS SELECT toStartOfHour(ts) AS hour, count() AS cnt FROM events GROUP BY hour;`

	got := stripDumpedViewHeader(dumped)
	require.NotContains(t, got, "DEFINER")
	require.NotContains(t, got, "`hour` DateTime")
	require.Contains(t, got, "REFRESH EVERY 1 HOUR")
	require.Contains(t, got, "TO default.hourly_snapshot")
	require.Contains(t, got, "AS SELECT")

	_, err := parser.ParseString(got)
	require.NoError(t, err)
}

func TestStripDumpedViewHeaderLeavesPlainView(t *testing.T) {
	dumped := `CREATE VIEW default.events_batch
(
    ` + "`id`" + ` UInt64
)
AS SELECT * FROM (SELECT id FROM events LIMIT (SELECT n FROM caps));`

	got := stripDumpedViewHeader(dumped)
	require.Contains(t, got, "AS SELECT * FROM")
	require.NotContains(t, got, "`id` UInt64")

	_, err := parser.ParseString(got)
	require.NoError(t, err)
}

func TestStripDumpedViewHeaderDoesNotEatToFunction(t *testing.T) {
	dumped := `CREATE MATERIALIZED VIEW default.mv_hourly REFRESH EVERY 1 HOUR TO remote('h', default, hourly_snapshot) AS SELECT 1;`

	got := stripDumpedViewHeader(dumped)
	require.Contains(t, got, "TO remote('h', default, hourly_snapshot)")
	require.Contains(t, got, "REFRESH EVERY 1 HOUR")
}

func TestStripDumpedViewHeaderDoesNotEatEngineArgs(t *testing.T) {
	dumped := `CREATE MATERIALIZED VIEW db.mv_hourly_stats ENGINE = ReplicatedSummingMergeTree('/clickhouse/tables/{shard}/db/mv_hourly_stats', '{replica}') PARTITION BY toYYYYMM(hour) ORDER BY (hour, event_type) SETTINGS index_granularity = 8192 AS SELECT toStartOfHour(timestamp) AS hour FROM db.events;`

	got := stripDumpedViewHeader(dumped)
	require.Contains(t, got, "ENGINE = ReplicatedSummingMergeTree('/clickhouse/tables/{shard}/db/mv_hourly_stats', '{replica}')")
	require.Contains(t, got, "ORDER BY (hour, event_type)")
}

func TestStripDumpedViewHeaderRemovesColumnsAfterToRefreshAppend(t *testing.T) {
	dumped := `CREATE MATERIALIZED VIEW default.mv_hourly REFRESH EVERY 6 HOUR APPEND TO default.hourly_snapshot (` + "`name`" + ` String, ` + "`id`" + ` UInt64) AS SELECT name, id FROM events;`

	got := stripDumpedViewHeader(dumped)
	require.NotContains(t, got, "`name` String")
	require.Contains(t, got, "REFRESH EVERY 6 HOUR APPEND")
	require.Contains(t, got, "TO default.hourly_snapshot")
	require.Contains(t, got, "AS SELECT")

	_, err := parser.ParseString(got)
	require.NoError(t, err)
}

func TestStripDumpedViewHeaderRemovesColumnsAfterTo(t *testing.T) {
	dumped := `CREATE MATERIALIZED VIEW default.mv_hourly TO default.hourly_snapshot (` + "`hour`" + ` DateTime, ` + "`cnt`" + ` UInt64) AS SELECT toStartOfHour(ts) AS hour, count() AS cnt FROM events GROUP BY hour;`

	got := stripDumpedViewHeader(dumped)
	require.Contains(t, got, "TO default.hourly_snapshot")
	require.NotContains(t, got, "`hour` DateTime")
	require.Contains(t, got, "AS SELECT")

	_, err := parser.ParseString(got)
	require.NoError(t, err)
}
