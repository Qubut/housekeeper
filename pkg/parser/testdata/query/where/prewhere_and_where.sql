SELECT `id`
FROM `events`
PREWHERE `kind` IN ('sale', 'listing')
WHERE `ts` > now() - INTERVAL 1 DAY;
