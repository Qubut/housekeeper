CREATE MATERIALIZED VIEW `mv_dep`
REFRESH DEPENDS ON `db`.`mv_upstream`
TO `db`.`dep_target`
AS SELECT 1 AS `x`;
