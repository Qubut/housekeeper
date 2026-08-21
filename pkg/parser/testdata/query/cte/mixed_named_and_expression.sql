WITH
    `batch_caps` AS (
        SELECT
            `n_floor`,
            `n_value`
        FROM `slot_mix`
        WHERE `market` = 'items'
    ),
    (SELECT `n_floor` FROM `batch_caps`) AS `items_n_floor`
SELECT *
FROM `candidates`
LIMIT `items_n_floor`;
