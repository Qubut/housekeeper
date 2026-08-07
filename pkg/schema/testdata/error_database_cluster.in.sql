-- Current state: database without cluster configuration
CREATE DATABASE db ENGINE = Atomic COMMENT 'Sample database'
;
-- Target state: attempting to add cluster configuration (should fail)
CREATE DATABASE db ON CLUSTER production ENGINE = Atomic COMMENT 'Sample database';