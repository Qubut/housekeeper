CREATE DICTIONARY IF NOT EXISTS `default`.`catalog_items` ON CLUSTER '{cluster}' (
    `item_id`  UInt64,
    `exterior` Nullable(String),
    `rarity`   Nullable(String)
)
PRIMARY KEY `item_id`
SOURCE(POSTGRESQL(NAME postgres_catalog TABLE 'catalog_items' SCHEMA 'counter_strike' where '1' invalidate_query 'SELECT max(item_id) FROM counter_strike.catalog_items'))
LAYOUT(HASHED_ARRAY())
LIFETIME(MIN 300 MAX 600);
