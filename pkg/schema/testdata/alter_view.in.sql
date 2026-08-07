-- Current state: simple view and basic materialized view
CREATE VIEW db.stats AS SELECT count(*) AS total FROM events;
CREATE MATERIALIZED VIEW db.mv_stats ENGINE = MergeTree() ORDER BY date AS SELECT toDate(timestamp) AS date FROM events;
-- Target state: enhanced view with latest timestamp and materialized view with count aggregation
CREATE VIEW db.stats AS SELECT count(*) AS total, max(timestamp) AS latest FROM events;
CREATE MATERIALIZED VIEW db.mv_stats ENGINE = MergeTree() ORDER BY date AS SELECT toDate(timestamp) AS date, count() AS cnt FROM events GROUP BY date;