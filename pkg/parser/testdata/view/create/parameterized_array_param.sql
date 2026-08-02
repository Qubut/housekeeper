CREATE VIEW `v_buff163_poll_candidates`
AS SELECT
    `c`.`item_id` AS `item_id`,
    `w`.`last_polled` AS `last_polled`
FROM (SELECT arrayJoin({ids:Array(UInt64)}) AS `item_id`) AS `c`
LEFT JOIN (SELECT
    `item_id`,
    max(`last_updated`) AS `last_polled`
FROM `update_history`
WHERE `source` = 'buff163' AND `item_id` IN {ids:Array(UInt64)}
GROUP BY `item_id`) AS `w` ON `w`.`item_id` = `c`.`item_id`;
