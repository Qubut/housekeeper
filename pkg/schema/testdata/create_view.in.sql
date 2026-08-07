-- Current state: no views exist
;
-- Target state: create new regular view and materialized view
CREATE VIEW db.stats AS SELECT count(*) AS total FROM events;
CREATE MATERIALIZED VIEW db.mv_stats ENGINE = MergeTree() ORDER BY date AS SELECT toDate(timestamp) AS date, count() AS cnt FROM events GROUP BY date;