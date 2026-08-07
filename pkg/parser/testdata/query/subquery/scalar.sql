SELECT
    `id`,
    (SELECT max(`n`)
FROM `batch_caps`) AS `cap`
FROM `items`;
