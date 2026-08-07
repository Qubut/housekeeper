ALTER TABLE `db`.`events`
    MOVE PARTITION '202301' TO TABLE `db`.`events_archive`;
