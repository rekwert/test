-- Enforce balance ↔ balance_kopecks sync at DB level.
ALTER TABLE billing.accounts
    DROP CONSTRAINT IF EXISTS accounts_balance_kopecks_sync;

ALTER TABLE billing.accounts
    ADD CONSTRAINT accounts_balance_kopecks_sync
    CHECK (balance IS NOT DISTINCT FROM (balance_kopecks::numeric / 100));
