CREATE MATERIALIZED VIEW `analytics`.`mv_hourly`
REFRESH EVERY 1 HOUR
TO `analytics`.`hourly_snapshot`
AS SELECT
    toStartOfHour(`ts`) AS `hour`,
    count() AS `cnt`
FROM `events`
GROUP BY `hour`;
