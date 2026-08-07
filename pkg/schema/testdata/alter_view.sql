DROP TABLE `db`.`mv_stats`;

CREATE MATERIALIZED VIEW `db`.`mv_stats`
ENGINE = MergeTree() ORDER BY `date`
AS SELECT
    toDate(`timestamp`) AS `date`,
    count() AS `cnt`
FROM `events`
GROUP BY `date`;

CREATE OR REPLACE VIEW `db`.`stats`
AS SELECT
    count(*) AS `total`,
    max(`timestamp`) AS `latest`
FROM `events`;