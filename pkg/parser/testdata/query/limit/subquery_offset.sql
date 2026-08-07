SELECT `id`
FROM `items`
LIMIT (SELECT `n`
FROM `batch_caps`
WHERE `name` = 'default') OFFSET 0;
