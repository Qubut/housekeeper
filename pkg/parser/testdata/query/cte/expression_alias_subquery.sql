WITH
    (SELECT sum(`n`) FROM numbers(10)) AS `total`
SELECT `total`;
