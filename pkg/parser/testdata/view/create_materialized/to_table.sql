CREATE MATERIALIZED VIEW `mv_to_table`
TO `db`.`target_table`
AS SELECT *
FROM `source_table`
WHERE `status` = 'active';
