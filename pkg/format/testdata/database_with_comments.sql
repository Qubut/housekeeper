-- Create the sample database

CREATE DATABASE `db` ENGINE = Atomic COMMENT 'Sample database';

-- Alter the database to update the comment

ALTER DATABASE `db` MODIFY COMMENT 'Updated sample database';

-- Attach a database
-- This is useful for recovery scenarios

ATTACH DATABASE IF NOT EXISTS `backup` ENGINE = Memory ON CLUSTER `production`;

-- Detach the database temporarily  

DETACH DATABASE IF EXISTS `temp_db` ON CLUSTER `production` PERMANENTLY;

-- Drop the old database
-- Be careful with this operation!

DROP DATABASE IF EXISTS `old_db` ON CLUSTER `production` SYNC;

-- Rename databases
-- This is a batch operation

RENAME DATABASE `staging` TO `production`, `test` TO `staging` ON CLUSTER `main`;
