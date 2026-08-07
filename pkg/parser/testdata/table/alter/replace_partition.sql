ALTER TABLE `db`.`events`
    REPLACE PARTITION '202301' FROM `db`.`events_backup`;
