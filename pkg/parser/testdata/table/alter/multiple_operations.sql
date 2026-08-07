ALTER TABLE `db`.`events`
    ADD COLUMN `session_id` UUID,
    DROP COLUMN `tags`,
    RENAME COLUMN `data` TO `event_data`,
    COMMENT COLUMN `timestamp` ;
