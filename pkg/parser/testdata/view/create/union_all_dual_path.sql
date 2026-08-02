CREATE VIEW `v_groups` ON CLUSTER '{cluster}'
AS SELECT
    ifNull(`bucket_group_id`, '') AS `g_id`,
    any(toUInt64(`item_id`)) AS `item_id`,
    any(`name`) AS `name`
FROM (SELECT
    `item_id`,
    `name`,
    `bucket_group_id`
FROM dictionary(`catalog`)
WHERE `name` != '' AND ifNull(`bucket_group_id`, '') != '')
GROUP BY `g_id`
UNION ALL
SELECT
    `name` AS `g_id`,
    toUInt64(`item_id`) AS `item_id`,
    `name`
FROM dictionary(`catalog`)
WHERE `name` != '' AND ifNull(`bucket_group_id`, '') = '';
