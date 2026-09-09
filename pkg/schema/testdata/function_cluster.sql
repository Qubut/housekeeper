DROP FUNCTION IF EXISTS `local_function`;

CREATE FUNCTION `local_function` ON CLUSTER `production` AS (`x`) -> multiply(`x`, 2);