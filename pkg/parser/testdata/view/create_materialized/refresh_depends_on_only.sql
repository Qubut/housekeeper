CREATE MATERIALIZED VIEW `mv_dep`
REFRESH DEPENDS ON `analytics`.`mv_upstream`
TO `analytics`.`dep_target`
AS SELECT 1 AS `x`;
