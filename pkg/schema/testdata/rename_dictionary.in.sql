-- Current state: dictionary with old name
CREATE DICTIONARY db.old_users_dict (id UInt64, name String) PRIMARY KEY id SOURCE(HTTP(url 'http://api.com/users')) LAYOUT(HASHED()) LIFETIME(3600)
;
-- Target state: same dictionary with new name
CREATE DICTIONARY db.users_dict (id UInt64, name String) PRIMARY KEY id SOURCE(HTTP(url 'http://api.com/users')) LAYOUT(HASHED()) LIFETIME(3600);