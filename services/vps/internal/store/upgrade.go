package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrSamePlan             = errors.New("same plan")
	ErrDowngradeNotAllowed  = errors.New("downgrade not allowed")
	ErrDiskShrinkNotAllowed = errors.New("disk shrink not allowed")
	ErrDifferentPlanLine    = errors.New("different plan line")
	ErrPlanRegionMismatch   = errors.New("plan region mismatch")
)

func planLine(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(n, "prosto"):
		return "prosto"
	case strings.HasPrefix(n, "midrange"):
		return "midrange"
	case strings.HasPrefix(n, "hustle"):
		return "hustle"
	case strings.HasPrefix(n, "custom"):
		return "custom"
	case strings.HasPrefix(n, "storage"):
		return "custom"
	default:
		return ""
	}
}

type UpgradeResult struct {
	Instance   *Instance
	Amount     float64
	FromPlan   string
	ToPlan     string
	FromPlanID string
	ToPlanID   string
}

type upgradeCalc struct {
	OldPlanID string
	NewPlanID string
	OldName   string
	NewName   string
	OldPrice  float64
	NewPrice  float64
	Amount    float64
	RemainingDays float64
	PeriodDays    int
	NextBilling   *time.Time
	Hostname  string
	Region    string
}

func (s *Store) loadUpgradeCalc(ctx context.Context, userID, instanceID, newPlanID string) (*upgradeCalc, error) {
	var (
		oldPlanID                    string
		oldName, newName             string
		oldPrice, newPrice           float64
		oldCPU, newCPU, oldRAM, newRAM int
		oldDisk, newDisk, periodDays int
		hostname                     *string
		nextBilling                  *time.Time
		createdAt                    time.Time
		region                       string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT
			i.plan_id::text,
			i.region,
			i.hostname,
			i.next_billing_at,
			i.created_at,
			COALESCE(NULLIF(i.billing_period_days, 0), 30),
			COALESCE(op.name, ''), COALESCE(op.price_monthly, 0)::float8,
			COALESCE(op.cpu, 0), COALESCE(op.ram_mb, 0), COALESCE(op.disk_gb, 0),
			COALESCE(np.name, ''), COALESCE(np.price_monthly, 0)::float8,
			COALESCE(np.cpu, 0), COALESCE(np.ram_mb, 0), COALESCE(np.disk_gb, 0)
		FROM vps.instances i
		JOIN vps.plans op ON op.id = i.plan_id
		JOIN vps.plans np ON np.id = $3::uuid AND np.active = true AND np.region = i.region
		WHERE i.id = $1::uuid AND i.user_id = $2::uuid AND i.state <> 'deleted'
	`, instanceID, userID, newPlanID).Scan(
		&oldPlanID, &region, &hostname, &nextBilling, &createdAt, &periodDays,
		&oldName, &oldPrice, &oldCPU, &oldRAM, &oldDisk,
		&newName, &newPrice, &newCPU, &newRAM, &newDisk,
	)
	if err == pgx.ErrNoRows {
		var instRegion, newRegion string
		_ = s.pool.QueryRow(ctx, `
			SELECT COALESCE(region, '') FROM vps.instances
			WHERE id = $1::uuid AND user_id = $2::uuid AND state <> 'deleted'
		`, instanceID, userID).Scan(&instRegion)
		if regErr := s.pool.QueryRow(ctx, `
			SELECT COALESCE(region, '') FROM vps.plans WHERE id = $1::uuid AND active = true
		`, newPlanID).Scan(&newRegion); regErr == nil &&
			instRegion != "" && newRegion != "" && instRegion != newRegion {
			return nil, ErrPlanRegionMismatch
		}
		return nil, ErrInstanceNotFound
	}
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(oldPlanID, newPlanID) {
		return nil, ErrSamePlan
	}
	if ol, nl := planLine(oldName), planLine(newName); ol == "" || nl == "" || ol != nl {
		return nil, ErrDifferentPlanLine
	}
	if newDisk < oldDisk {
		return nil, ErrDiskShrinkNotAllowed
	}
	if newPrice < oldPrice {
		return nil, ErrDowngradeNotAllowed
	}
	if newPrice == oldPrice && newCPU <= oldCPU && newRAM <= oldRAM && newDisk <= oldDisk {
		return nil, ErrDowngradeNotAllowed
	}

	delta := newPrice - oldPrice
	now := time.Now().UTC()
	remainingDays := calcUpgradeRemainingDays(now, nextBilling, createdAt, periodDays)
	amount := calcProratedUpgradeAmount(delta, remainingDays, periodDays)

	hostLabel := newName
	if hostname != nil && *hostname != "" {
		hostLabel = *hostname
	}
	return &upgradeCalc{
		OldPlanID: oldPlanID,
		NewPlanID: newPlanID,
		OldName:   oldName,
		NewName:   newName,
		OldPrice:  oldPrice,
		NewPrice:  newPrice,
		Amount:    amount,
		RemainingDays: remainingDays,
		PeriodDays:    periodDays,
		NextBilling:   nextBilling,
		Hostname:  hostLabel,
		Region:    region,
	}, nil
}

// GetUpgradeQuoteForUser returns prorated upgrade pricing without mutating state.
func (s *Store) GetUpgradeQuoteForUser(ctx context.Context, userID, instanceID, newPlanID string) (*UpgradeQuote, error) {
	calc, err := s.loadUpgradeCalc(ctx, userID, instanceID, newPlanID)
	if err != nil {
		return nil, err
	}
	return &UpgradeQuote{
		Amount:            calc.Amount,
		DeltaMonthly:      calc.NewPrice - calc.OldPrice,
		RemainingDays:     calc.RemainingDays,
		BillingPeriodDays: calc.PeriodDays,
		NextBillingAt:     calc.NextBilling,
		FromPlan:          calc.OldName,
		ToPlan:            calc.NewName,
		FromPlanID:        calc.OldPlanID,
		ToPlanID:          calc.NewPlanID,
	}, nil
}

// ValidateUpgradeForUser checks plan rules and balance without mutating state.
func (s *Store) ValidateUpgradeForUser(ctx context.Context, userID, instanceID, newPlanID string) (amount float64, err error) {
	calc, err := s.loadUpgradeCalc(ctx, userID, instanceID, newPlanID)
	if err != nil {
		return 0, err
	}
	if calc.Amount <= 0 {
		return 0, nil
	}
	var billingStatus string
	var balance float64
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(a.billing_status, 'active'), COALESCE(a.balance, 0)::float8
		FROM billing.accounts a
		WHERE a.user_id = $1::uuid
	`, userID).Scan(&billingStatus, &balance)
	if err == pgx.ErrNoRows {
		return 0, ErrInsufficientBalance
	}
	if err != nil {
		return 0, err
	}
	if billingStatus == "suspended" {
		return 0, ErrBillingSuspended
	}
	if balance < calc.Amount {
		return 0, ErrInsufficientBalance
	}
	return calc.Amount, nil
}

// UpgradeInstanceForUser charges the prorated price delta and updates plan_id.
// Caller must resize the hypervisor after a successful charge; on resize failure
// call RevertUpgradeInstance and roll back the hypervisor package.
func (s *Store) UpgradeInstanceForUser(ctx context.Context, userID, instanceID, newPlanID, staffID string) (*UpgradeResult, error) {
	calc, err := s.loadUpgradeCalc(ctx, userID, instanceID, newPlanID)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.accounts (user_id) VALUES ($1::uuid) ON CONFLICT DO NOTHING
	`, userID); err != nil {
		return nil, err
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
		return nil, err
	}
	if billingStatus == "suspended" {
		return nil, ErrBillingSuspended
	}
	if calc.Amount > 0 && balance < calc.Amount {
		return nil, ErrInsufficientBalance
	}

	var balanceAfter float64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(balance, 0)::float8 FROM billing.accounts WHERE user_id = $1::uuid
	`, userID).Scan(&balanceAfter); err != nil {
		return nil, err
	}
	if calc.Amount > 0 {
		if err := tx.QueryRow(ctx, `
			UPDATE billing.accounts
			SET balance = balance - $2, updated_at = now()
			WHERE user_id = $1::uuid
			RETURNING balance::float8
		`, userID, calc.Amount).Scan(&balanceAfter); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances
		SET plan_id = $2::uuid, updated_at = now()
		WHERE id = $1::uuid
	`, instanceID, newPlanID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO vps.instance_addons (instance_id, addon_type, status)
		VALUES ($1::uuid, 'plan_upgrade', 'applied')
	`, instanceID); err != nil {
		return nil, err
	}

	desc := fmt.Sprintf("VPS upgrade — %s: %s → %s", calc.Hostname, calc.OldName, calc.NewName)
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, instance_id, balance_after)
		VALUES ($1::uuid, $2, 'paid', $3, 'balance', 'charge', $4::uuid, $5)
	`, userID, calc.Amount, desc, instanceID, balanceAfter); err != nil {
		return nil, err
	}

	if calc.Amount > 0 {
		if err := s.processReferralPayment(ctx, tx, userID, calc.Amount); err != nil {
			return nil, err
		}
	}

	details, _ := json.Marshal(map[string]any{
		"from_plan_id": calc.OldPlanID,
		"to_plan_id":   newPlanID,
		"from_plan":    calc.OldName,
		"to_plan":      calc.NewName,
		"amount":       calc.Amount,
		"remaining_days": calc.RemainingDays,
		"billing_period_days": calc.PeriodDays,
		"region":       calc.Region,
	})
	if err := insertAdminAction(ctx, tx, staffID, userID, instanceID, "instance_upgrade", details); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	inst, err := s.GetInstanceForUser(ctx, userID, instanceID)
	if err != nil {
		return nil, err
	}
	return &UpgradeResult{
		Instance:   inst,
		Amount:     calc.Amount,
		FromPlan:   calc.OldName,
		ToPlan:     calc.NewName,
		FromPlanID: calc.OldPlanID,
		ToPlanID:   newPlanID,
	}, nil
}

// RevertUpgradeInstance restores the previous plan and refunds a failed upgrade charge.
func (s *Store) RevertUpgradeInstance(ctx context.Context, userID, instanceID, oldPlanID string, amount float64, reason string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE vps.instances
		SET plan_id = $2::uuid, updated_at = now()
		WHERE id = $1::uuid AND user_id = $3::uuid
	`, instanceID, oldPlanID, userID); err != nil {
		return err
	}
	if amount > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO billing.accounts (user_id) VALUES ($1::uuid) ON CONFLICT DO NOTHING
		`, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE billing.accounts
			SET balance = balance + $2, updated_at = now()
			WHERE user_id = $1::uuid
		`, userID, amount); err != nil {
			return err
		}
		desc := reason
		if desc == "" {
			desc = "VPS upgrade refund (hypervisor resize failed)"
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO billing.adjustments (user_id, amount, kind, reason)
			VALUES ($1::uuid, $2, 'credit', $3)
		`, userID, amount, desc); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
