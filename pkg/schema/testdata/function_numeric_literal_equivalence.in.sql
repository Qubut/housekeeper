-- Current state: ClickHouse create_query scientific / trimmed floats
CREATE FUNCTION scale_unit AS (x) -> greatest(x, 1e-9);
CREATE FUNCTION apply_rate AS (amount) -> multiply(amount, 0.2);
-- Target state: source DDL decimal spelling of the same values
CREATE FUNCTION scale_unit AS (x) -> greatest(x, 0.000000001);
CREATE FUNCTION apply_rate AS (amount) -> multiply(amount, 0.20);
