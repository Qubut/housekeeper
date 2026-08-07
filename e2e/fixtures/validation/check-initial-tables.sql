-- Verify initial tables exist
SELECT 'users_table' as check_name, count(*) as result
FROM system.tables
WHERE database = 'db' AND name = 'users'
HAVING result = 1;