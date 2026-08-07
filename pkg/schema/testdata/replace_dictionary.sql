CREATE OR REPLACE DICTIONARY `db`.`users_dict` (
    `id`    UInt64,
    `name`  String,
    `email` String DEFAULT ''
)
PRIMARY KEY `id`
SOURCE(HTTP(url 'http://api.com/users'))
LAYOUT(HASHED())
LIFETIME(7200);