package store

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
)

var ErrInstanceNotFound = fmt.Errorf("instance not found")

// ExtendInstanceMonths charges the user balance for N months at plan price and
// pushes next_billing_at by months * billing_period_days (default 30).
func (s *Store) ExtendInstanceMonths(ctx context.Context, userID, instanceID string, months int) (*Instance, float64, error) {
	if months < 1 || months > 36 {
		return nil, 0, ErrInvalidPeriod
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx)

	var planName string
	var priceMonthly float64
	var periodDays int
	var hostname *string
	var noRenew bool
	var isFreeWeek bool
	err = tx.QueryRow(ctx, `
		SELECT
			COALESCE(p.name, ''),
			COALESCE(p.price_monthly, 0)::float8,
			COALESCE(NULLIF(i.billing_period_days, 0), 30),
			i.hostname,
			COALESCE((i.provider_meta->>'admin_issued')::boolean, false)
			    OR COALESCE((i.provider_meta->>'no_renew')::boolean, false),
			COALESCE((i.provider_meta->>'free_week')::boolean, false)
			    OR COALESCE((i.provider_meta->>'trial')::boolean, false)
		FROM vps.instances i
		JOIN vps.plans p ON p.id = i.plan_id
		WHERE i.id = $1::uuid AND i.user_id = $2::uuid AND i.state <> 'deleted'
		FOR UPDATE OF i
	`, instanceID, userID).Scan(&planName, &priceMonthly, &periodDays, &hostname, &noRenew, &isFreeWeek)
	if err == pgx.ErrNoRows {
		return nil, 0, ErrInstanceNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	if noRenew {
		return nil, 0, fmt.Errorf("renewal not allowed for this instance")
	}

	convertTrial := isFreeWeek && !noRenew
	if convertTrial {
		periodDays = 30
	}

	amount := math.Round(priceMonthly*float64(months)*100) / 100
	extendDays := months * periodDays

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.accounts (user_id) VALUES ($1::uuid) ON CONFLICT DO NOTHING
	`, userID); err != nil {
		return nil, 0, err
	}

	var billingStatus string
	var balance float64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(a.billing_status, 'active'), COALESCE(a.balance, 0)::float8
		FROM billing.accounts a
		WHERE a.user_id = $1::uuid
		FOR UPDATE
	`, userID).Scan(&billingStatus, &balance)
	if err != nil {
		return nil, 0, err
	}
	if billingStatus == "suspended" {
		return nil, 0, ErrBillingSuspended
	}
	if amount > 0 && balance < amount {
		return nil, 0, ErrInsufficientBalance
	}

	balanceAfter := balance
	if amount > 0 {
		if err := tx.QueryRow(ctx, `
			UPDATE billing.accounts
			SET balance = balance - $2, updated_at = now()
			WHERE user_id = $1::uuid
			RETURNING balance::float8
		`, userID, amount).Scan(&balanceAfter); err != nil {
			return nil, 0, err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances
		SET next_billing_at = GREATEST(COALESCE(next_billing_at, now()), now())
			+ make_interval(days => $3),
			billing_status = 'active',
			suspended_at = NULL,
			billing_period_days = CASE WHEN $4::boolean THEN 30 ELSE billing_period_days END,
			auto_renew = CASE WHEN $4::boolean THEN true ELSE auto_renew END,
			provider_meta = CASE
			  WHEN $4::boolean AND COALESCE((provider_meta->>'free_week')::boolean, false) THEN
			    COALESCE(provider_meta, '{}'::jsonb)
			        || '{"converted_to_paid": true, "initial_prepaid_days": 30}'::jsonb
			  ELSE provider_meta
			END,
			state = CASE
				WHEN state IN ('stopped', 'suspended') THEN 'starting'
				ELSE state
			END,
			updated_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid
	`, instanceID, userID, extendDays, convertTrial); err != nil {
		return nil, 0, err
	}

	hostLabel := planName
	if hostname != nil && *hostname != "" {
		hostLabel = *hostname
	}
	desc := fmt.Sprintf("VPS renewal — %s (%d mo.)", hostLabel, months)
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, instance_id, balance_after)
		VALUES ($1::uuid, $2, 'paid', $3, 'balance', 'charge', $4::uuid, $5)
	`, userID, amount, desc, instanceID, balanceAfter); err != nil {
		return nil, 0, err
	}

	if amount > 0 {
		if err := s.processReferralPayment(ctx, tx, userID, amount); err != nil {
			return nil, 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, 0, err
	}

	inst, err := s.GetInstanceForUser(ctx, userID, instanceID)
	if err != nil {
		return nil, 0, err
	}
	if inst.State == "starting" {
		externalID, _ := s.GetInstanceExternalID(ctx, instanceID)
		_ = s.EnqueueInstanceStart(ctx, instanceID, externalID, userID)
	}
	return inst, amount, nil
}
