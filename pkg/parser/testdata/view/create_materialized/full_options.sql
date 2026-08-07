CREATE OR REPLACE MATERIALIZED VIEW IF NOT EXISTS `db`.`mv_complex` ON CLUSTER `production`
TO `db`.`destination_table`
ENGINE = ReplacingMergeTree(`version`)
POPULATE
AS SELECT
    `id`,
    `name`,
    max(`version`) AS `version`,
    argMax(`data`, `version`) AS `data`
FROM `source`
GROUP BY `id`, `name`;
