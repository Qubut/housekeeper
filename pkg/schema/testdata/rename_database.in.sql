-- Current state: database with old name
CREATE DATABASE old_db ENGINE = Atomic COMMENT 'Sample database';
-- Target state: same database with new name
CREATE DATABASE db ENGINE = Atomic COMMENT 'Sample database';