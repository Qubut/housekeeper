-- Database operations with various features
CREATE DATABASE db ENGINE = Atomic COMMENT 'Sample database';

CREATE DATABASE IF NOT EXISTS warehouse ON CLUSTER production ENGINE = MaterializedMySQL('localhost:3306', 'warehouse', 'user', 'password') COMMENT 'Data warehouse';

ALTER DATABASE db MODIFY COMMENT 'Updated sample database';

RENAME DATABASE old_db TO db, temp_warehouse TO warehouse;

DROP DATABASE IF EXISTS legacy_db ON CLUSTER production SYNC;