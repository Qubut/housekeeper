ALTER TABLE `db`.`events`
    MODIFY TTL `timestamp` + days(30);
