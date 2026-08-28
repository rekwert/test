CREATE SEQUENCE IF NOT EXISTS billing.robokassa_inv_seq START WITH 100000;

ALTER TABLE billing.invoices
  ADD COLUMN IF NOT EXISTS robokassa_inv_id BIGINT UNIQUE;

CREATE INDEX IF NOT EXISTS invoices_robokassa_inv_idx
  ON billing.invoices (robokassa_inv_id)
  WHERE robokassa_inv_id IS NOT NULL;
