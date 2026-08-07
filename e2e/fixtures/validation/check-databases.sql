-- Verify required databases exist
SELECT 'sample_database' as check_name, count(*) as result
FROM system.databases 
WHERE name = 'db'
HAVING result = 1;