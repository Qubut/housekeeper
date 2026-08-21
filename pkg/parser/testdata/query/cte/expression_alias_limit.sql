WITH
    (SELECT `n_floor` FROM `slot_mix` WHERE `market` = 'items') AS `items_n_floor`,
    (SELECT `n_value` FROM `slot_mix` WHERE `market` = 'items') AS `items_n_value`
SELECT *
FROM (SELECT `id`
FROM `candidates`
WHERE `due` = 1
ORDER BY `score` DESC
LIMIT `items_n_floor`)
UNION ALL
SELECT *
FROM (SELECT `id`
FROM `candidates`
WHERE `due` = 1
ORDER BY `score` DESC
LIMIT `items_n_value`);
