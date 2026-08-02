CREATE VIEW `v_by_id`
AS SELECT
    `id`,
    `name`
FROM `items`
WHERE `id` = {id:UInt64};
