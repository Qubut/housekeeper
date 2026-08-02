SELECT
    `item_id`,
    `ts`,
    sum(`x`) OVER (PARTITION BY item_id ORDER BY ts ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS `running_sum`
FROM `events`;
