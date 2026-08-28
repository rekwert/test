-- Human-readable sequential order numbers (keep UUID id as PK).

ALTER TABLE vps.orders
    ADD COLUMN IF NOT EXISTS order_number BIGINT;

CREATE SEQUENCE IF NOT EXISTS vps.orders_order_number_seq;

-- Backfill existing rows in creation order
WITH numbered AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY created_at ASC, id ASC) AS n
    FROM vps.orders
    WHERE order_number IS NULL
)
UPDATE vps.orders o
SET order_number = numbered.n
FROM numbered
WHERE o.id = numbered.id;

-- Align sequence with current max
SELECT setval(
    'vps.orders_order_number_seq',
    GREATEST(COALESCE((SELECT MAX(order_number) FROM vps.orders), 1), 1),
    true
);

ALTER TABLE vps.orders
    ALTER COLUMN order_number SET DEFAULT nextval('vps.orders_order_number_seq');

ALTER SEQUENCE vps.orders_order_number_seq OWNED BY vps.orders.order_number;

UPDATE vps.orders
SET order_number = nextval('vps.orders_order_number_seq')
WHERE order_number IS NULL;

ALTER TABLE vps.orders
    ALTER COLUMN order_number SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS orders_order_number_uidx
    ON vps.orders (order_number);
