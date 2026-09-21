CREATE VIEW `events_recent`
AS SELECT `id`
FROM `events`
PREWHERE `kind` = 'sale'
WHERE `ts` > now() - INTERVAL 1 DAY;
