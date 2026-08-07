SELECT
    `id`,
    `name`
FROM `items`
ORDER BY `score` DESC
LIMIT (SELECT `n`
FROM `batch_caps`
WHERE `name` = 'default');
