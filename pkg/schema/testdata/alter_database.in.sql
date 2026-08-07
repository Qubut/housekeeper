-- Current state: database with old comment
CREATE DATABASE db ENGINE = Atomic COMMENT 'Old comment';
-- Target state: same database with updated comment  
CREATE DATABASE db ENGINE = Atomic COMMENT 'Updated comment';