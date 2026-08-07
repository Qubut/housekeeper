CREATE MATERIALIZED VIEW `db`.`mv_hourly`
REFRESH EVERY 1 HOUR
TO `db`.`hourly_snapshot`
AS SELECT
    toStartOfHour(`ts`) AS `hour`,
    count() AS `cnt`
FROM `events`
GROUP BY `hour`;
