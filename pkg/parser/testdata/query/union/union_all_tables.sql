SELECT
    `id`,
    `name`
FROM `skins`
WHERE `active` = 1
UNION ALL
SELECT
    `id`,
    `name`
FROM `stickers`
WHERE `active` = 1;
