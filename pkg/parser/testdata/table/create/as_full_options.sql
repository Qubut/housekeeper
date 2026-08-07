CREATE OR REPLACE TABLE IF NOT EXISTS `db`.`events_all` ON CLUSTER `db_cluster` AS `db`.`events_local`
ENGINE = Distributed(`db_cluster`, `db`, `events_local`, cityHash64(`user_id`))
SETTINGS index_granularity = 8192
COMMENT 'Distributed view of events_local';
