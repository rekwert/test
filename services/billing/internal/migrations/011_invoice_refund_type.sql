-- Allow refund invoice rows (also applied in vps 038 for shared DB).
ALTER TABLE billing.invoices
    DROP CONSTRAINT IF EXISTS invoices_invoice_type_check;
ALTER TABLE billing.invoices
    ADD CONSTRAINT invoices_invoice_type_check
    CHECK (invoice_type IN ('topup', 'charge', 'refund'));

DROP TABLE IF EXISTS billing.outbox;
