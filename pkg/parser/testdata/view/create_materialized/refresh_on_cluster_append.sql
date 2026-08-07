CREATE MATERIALIZED VIEW `analytics`.`mv_refresh` ON CLUSTER '{cluster}'
REFRESH EVERY 30 SECONDS APPEND
TO `analytics`.`target_table`
AS SELECT 1 AS `x`;
