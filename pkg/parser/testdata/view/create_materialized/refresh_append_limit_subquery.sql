CREATE MATERIALIZED VIEW `mv_batch` ON CLUSTER '{cluster}'
REFRESH EVERY 30 SECOND APPEND
TO `sink`
AS SELECT
    `id`,
    `score`
FROM `candidates`
ORDER BY `score` DESC
LIMIT (SELECT `n`
FROM `batch_caps`
WHERE `name` = 'default');
