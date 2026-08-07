-- Current state: database with Atomic engine
CREATE DATABASE db ENGINE = Atomic COMMENT 'Sample database';
-- Target state: attempting to change engine to Memory (should fail)
CREATE DATABASE db ENGINE = Memory COMMENT 'Sample database';