package store

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5"
)

// claimChargeDiscountTx atomically locks and consumes one promo entitlement row.
func (s *Store) claimChargeDiscountTx(ctx context.Context, tx pgx.Tx, userID, instanceID string) (float64, error) {
	var discount float64
	err := tx.QueryRow(ctx, `
		UPDATE billing.promo_entitlements
		SET active = false
		WHERE id = (
			SELECT id FROM billing.promo_entitlements
			WHERE user_id = $1::uuid AND active = true
			  AND discount_percent > 0
			  AND (expires_at IS NULL OR expires_at > now())
			  AND (instance_id IS NULL OR NULLIF($2, '') IS NULL OR instance_id = NULLIF($2, '')::uuid)
			ORDER BY discount_percent DESC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING discount_percent::float8
	`, userID, instanceID).Scan(&discount)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return discount, nil
}

func applyChargeDiscount(amount, discountPercent float64) float64 {
	if discountPercent <= 0 || amount <= 0 {
		return amount
	}
	out := math.Round(amount*(1-discountPercent/100)*100) / 100
	if out < 0 {
		return 0
	}
	return out
}

func (s *Store) UserPostTrialDiscountPercent(ctx context.Context, userID string) (float64, error) {
	var discount float64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(discount_percent), 0)::float8
		FROM billing.promo_entitlements
		WHERE user_id = $1::uuid AND active = true
		AND (expires_at IS NULL OR expires_at > now())
		AND instance_id IS NULL
	`, userID).Scan(&discount)
	return discount, err
}
