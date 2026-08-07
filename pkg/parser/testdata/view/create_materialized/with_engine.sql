CREATE MATERIALIZED VIEW `db`.`mv_aggregated`
ENGINE = MergeTree() ORDER BY (`date`, `user_id`)
AS SELECT
    toDate(`timestamp`) AS `date`,
    `user_id`,
    count() AS `events_count`
FROM `events`
GROUP BY `date`, `user_id`;
