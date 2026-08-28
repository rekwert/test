-- One-time repair: admin credits and legacy paths updated balance (numeric) while
-- client API reads balance_kopecks. Re-derive kopecks from balance where they diverge.
UPDATE billing.accounts
SET balance_kopecks = ROUND(balance * 100)::bigint
WHERE balance IS DISTINCT FROM (balance_kopecks::numeric / 100);
