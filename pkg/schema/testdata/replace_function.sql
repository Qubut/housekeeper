DROP FUNCTION IF EXISTS `calculate_tax`;

CREATE FUNCTION `calculate_tax` AS (`amount`) -> multiply(`amount`, 0.10);

DROP FUNCTION IF EXISTS `format_currency`;

CREATE FUNCTION `format_currency` AS (`value`, `currency`) -> concat(`currency`, ' ', toString(`value`));