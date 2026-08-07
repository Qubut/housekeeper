-- Mixed DDL operations
CREATE DATABASE db ENGINE = Atomic;

CREATE TABLE db.raw_events (id UInt64, data String, ts DateTime) ENGINE = MergeTree() ORDER BY ts;

CREATE DICTIONARY db.lookup (key UInt64, value String) PRIMARY KEY key SOURCE(HTTP(url 'http://api.test.com/data')) LAYOUT(HASHED()) LIFETIME(3600);

CREATE MATERIALIZED VIEW db.processed_events ENGINE = MergeTree() ORDER BY (date, id) AS SELECT id, JSONExtractString(data, 'event') AS event_type, toDate(ts) AS date FROM db.raw_events WHERE event_type != '';

ALTER TABLE db.raw_events ADD COLUMN processed UInt8 DEFAULT 0;

RENAME TABLE db.raw_events TO db.events_raw;

DROP VIEW IF EXISTS db.old_view;