-- Main schema file for E2E testing
-- This will be used as the baseline schema

-- Sample database will be created via migrations
CREATE DATABASE IF NOT EXISTS db ENGINE = Atomic COMMENT 'E2E test sample database';