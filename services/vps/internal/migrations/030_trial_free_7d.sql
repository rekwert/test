-- Free Trial: 7 days prepaid, zero list price (order path also forces amount=0).
UPDATE vps.plans
SET price_monthly = 0
WHERE tier = 'trial' AND active = true;
