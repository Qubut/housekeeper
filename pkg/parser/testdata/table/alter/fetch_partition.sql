ALTER TABLE `db`.`events`
    FETCH PARTITION '202301' FROM '/clickhouse/tables/events';
