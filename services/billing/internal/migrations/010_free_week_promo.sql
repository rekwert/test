-- System promo for free-week to paid conversion (10% first month, any tariff).
INSERT INTO billing.promo_codes (
    code, kind, value, min_amount, max_redemptions, per_user_limit,
    active, description
)
SELECT
    'FREEWEEK10',
    'charge_discount_percent',
    10,
    0,
    NULL,
    1,
    true,
    '10% off first paid month after PROSTO-1 free week'
WHERE NOT EXISTS (
    SELECT 1 FROM billing.promo_codes WHERE lower(code) = 'freeweek10'
);