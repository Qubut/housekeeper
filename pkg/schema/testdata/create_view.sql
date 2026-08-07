CREATE MATERIALIZED VIEW `db`.`mv_stats`
ENGINE = MergeTree() ORDER BY `date`
AS SELECT
    toDate(`timestamp`) AS `date`,
    count() AS `cnt`
FROM `events`
GROUP BY `date`;

CREATE VIEW `db`.`stats`
AS SELECT count(*) AS `total`
FROM `events`;