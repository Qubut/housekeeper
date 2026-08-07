CREATE VIEW `db`.`daily_summary`
AS SELECT
    `date`,
    count(*) AS `total`
FROM `events`
GROUP BY `date`;
