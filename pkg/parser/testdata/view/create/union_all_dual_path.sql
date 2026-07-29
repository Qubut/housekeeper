CREATE VIEW `v_steam_listing_groups` ON CLUSTER '{cluster}'
AS SELECT
    ifNull(`market_bucket_group_id`, '') AS `g_id`,
    any(toUInt64(`item_id`)) AS `item_id`,
    any(`market_hash_name`) AS `market_hash_name`
FROM (SELECT
    `item_id`,
    `market_hash_name`,
    `market_bucket_group_id`
FROM dictionary(`catalog_items`)
WHERE `market_hash_name` != '' AND ifNull(`market_bucket_group_id`, '') != '')
GROUP BY `g_id`
UNION ALL
SELECT
    `market_hash_name` AS `g_id`,
    toUInt64(`item_id`) AS `item_id`,
    `market_hash_name`
FROM dictionary(`catalog_items`)
WHERE `market_hash_name` != '' AND ifNull(`market_bucket_group_id`, '') = '';
