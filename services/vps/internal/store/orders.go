package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrBillingSuspended    = errors.New("billing account suspended")
	ErrPlanNotFound        = errors.New("plan not found")
	ErrInvalidPeriod       = errors.New("invalid period_months")
	ErrNoNodeForRegion     = errors.New("no online node for region")
	ErrTrialAlreadyUsed    = errors.New("trial already used") // free PROSTO-1 week already claimed
)

type CreateOrderInput struct {
	UserID            string
	PlanID            string
	Region            string
	Hostname          string
	RootPassword      string
	OSTemplateID      string
	SoftwareProfileID string
	PeriodMonths      int
	SSHKeyIDs         []string
	ProductType       string // vps | dedicated
	Provider          string // openstack | hetzner_robot
	ExternalProductID string
	ExtraIPv4Qty      int // dedicated only: additional IPv4 count (0..max)
}

type CreateOrderResult struct {
	OrderID     string
	OrderNumber int64
	InstanceID  string
	Amount      float64
	Status      string
	Queued      bool
}

func periodDiscount(months int) float64 {
	switch months {
	case 3:
		return 0.05
	case 6:
		return 0.10
	case 12:
		return 0.15
	default:
		return 0
	}
}

func (s *Store) CreateOrderWithBilling(ctx context.Context, in CreateOrderInput) (*CreateOrderResult, error) {
	if strings.EqualFold(in.ProductType, "dedicated") || IsDedicatedProvider(in.Provider) {
		return s.createDedicatedOrderWithBilling(ctx, in)
	}
	if in.PeriodMonths <= 0 {
		in.PeriodMonths = 1
	}
	switch in.PeriodMonths {
	case 1, 3, 6, 12:
	default:
		return nil, ErrInvalidPeriod
	}

	tier, err := s.ResolvePlanTier(ctx, in.PlanID)
	if err != nil {
		return nil, err
	}
	if tier == "" {
		return nil, ErrPlanNotFound
	}

	// Legacy Trial SKU is retired — block new orders.
	if strings.EqualFold(tier, "trial") {
		return nil, ErrPlanNotFound
	}

	nodeID, waitlisted, err := s.resolveOrderNode(ctx, in.Region, tier)
	if err != nil {
		return nil, err
	}

	sshKeys, err := s.ListSSHPublicKeys(ctx, in.UserID, in.SSHKeyIDs)
	if err != nil {
		return nil, err
	}
	sshKeysJSON, _ := json.Marshal(sshKeys)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var planName string
	var priceMonthly float64
	err = tx.QueryRow(ctx, `
		SELECT name, price_monthly::float8
		FROM vps.plans
		WHERE id = $1::uuid AND active = true
	`, in.PlanID).Scan(&planName, &priceMonthly)
	if err == pgx.ErrNoRows {
		return nil, ErrPlanNotFound
	}
	if err != nil {
		return nil, err
	}

	isFreeWeek := false
	if isProsto1PlanName(planName) {
		used, freeErr := userHasFreeWeekTx(ctx, tx, in.UserID)
		if freeErr != nil {
			return nil, freeErr
		}
		if !used {
			isFreeWeek = true
			in.PeriodMonths = 1
		}
	}

	discount := periodDiscount(in.PeriodMonths)
	amount := math.Round(priceMonthly*float64(in.PeriodMonths)*(1-discount)*100) / 100
	prepaidDays := in.PeriodMonths * 30
	autoRenew := true
	if isFreeWeek {
		amount = 0
		prepaidDays = 7
		autoRenew = false
		tag, err := tx.Exec(ctx, `
			INSERT INTO vps.free_week_claims (user_id) VALUES ($1::uuid) ON CONFLICT DO NOTHING
		`, in.UserID)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			return nil, ErrTrialAlreadyUsed
		}
	}

	var promoDiscount float64
	if !isFreeWeek && amount > 0 {
		promoDiscount, err = s.claimChargeDiscountTx(ctx, tx, in.UserID, "")
		if err != nil {
			return nil, err
		}
		if promoDiscount > 0 {
			amount = applyChargeDiscount(amount, promoDiscount)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.accounts (user_id) VALUES ($1::uuid) ON CONFLICT DO NOTHING
	`, in.UserID); err != nil {
		return nil, err
	}

	var billingStatus string
	var balance float64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(a.billing_status, 'active'), COALESCE(a.balance, 0)::float8
		FROM billing.accounts a
		WHERE a.user_id = $1::uuid
		FOR UPDATE
	`, in.UserID).Scan(&billingStatus, &balance)
	if err != nil {
		return nil, err
	}

	if billingStatus == "suspended" {
		return nil, ErrBillingSuspended
	}
	if balance < amount {
		return nil, ErrInsufficientBalance
	}

	orderID := uuid.New().String()
	instanceID := uuid.New().String()
	hostArg := strOrNil(in.Hostname)
	var rootPassArg any
	if in.RootPassword != "" {
		sealed, sealErr := s.sealSecret(in.RootPassword)
		if sealErr != nil {
			return nil, sealErr
		}
		rootPassArg = sealed
	}

	if amount > 0 {
		if err := tx.QueryRow(ctx, `
			UPDATE billing.accounts
			SET balance = balance - $2, updated_at = now()
			WHERE user_id = $1::uuid
			RETURNING balance::float8
		`, in.UserID, amount).Scan(&balance); err != nil {
			return nil, err
		}
	}

	var orderNumber int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO vps.orders (id, user_id, plan_id, region, status, os_template_id, software_profile_id, hostname)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'paid', $5, $6, $7)
		RETURNING order_number
	`, orderID, in.UserID, in.PlanID, in.Region, in.OSTemplateID, in.SoftwareProfileID, hostArg).Scan(&orderNumber); err != nil {
		return nil, err
	}

	instanceState := "creating"
	if waitlisted {
		instanceState = "queued"
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO vps.instances (
			id, user_id, order_id, plan_id, region, node_id, state, billing_status,
			hostname, root_password, billing_period_days, next_billing_at, provision_ssh_keys,
			auto_renew, provider_meta
		)
		VALUES (
			$1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, NULLIF($6, '')::uuid, $7, 'active',
			$8, $9, $11, NULL, $10::jsonb,
			$12, jsonb_build_object(
				'initial_prepaid_days', $11::int,
				'free_week', $13::boolean,
				'trial', $13::boolean
			)
		)
	`, instanceID, in.UserID, orderID, in.PlanID, in.Region, nodeID, instanceState, hostArg, rootPassArg, sshKeysJSON, prepaidDays, autoRenew, isFreeWeek); err != nil {
		return nil, err
	}

	tierLabel := strings.TrimSpace(tier)
	if tierLabel == "" {
		tierLabel = planName
	}
	desc := fmt.Sprintf("VPS · %s · %s · %dmo", in.Region, tierLabel, in.PeriodMonths)
	if isFreeWeek {
		desc = fmt.Sprintf("VPS · %s · PROSTO-1 · 7d free", in.Region)
	} else if promoDiscount > 0 {
		desc = fmt.Sprintf("%s (−%.0f%% post-trial)", desc, promoDiscount)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, instance_id, balance_after)
		VALUES ($1::uuid, $2, 'paid', $3, 'balance', 'charge', $4::uuid, $5)
	`, in.UserID, amount, desc, instanceID, balance); err != nil {
		return nil, err
	}

	if amount > 0 {
		if err := s.processReferralPayment(ctx, tx, in.UserID, amount); err != nil {
			return nil, err
		}
	}

	if !waitlisted {
		outboxPayload, _ := json.Marshal(map[string]any{
			"instance_id":         instanceID,
			"order_id":            orderID,
			"user_id":             in.UserID,
			"plan_id":             in.PlanID,
			"region":              in.Region,
			"node_id":             nodeID,
			"hostname":            in.Hostname,
			"os_template_id":      in.OSTemplateID,
			"software_profile_id": in.SoftwareProfileID,
			"ssh_keys":            sshKeys,
		})
		if _, err := insertProvisionOutboxTx(ctx, tx, outboxPayload); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &CreateOrderResult{
		OrderID:     orderID,
		OrderNumber: orderNumber,
		InstanceID:  instanceID,
		Amount:      amount,
		Status:      "paid",
		Queued:      waitlisted,
	}, nil
}

// UserHasFreeWeek reports whether the account already claimed the one-time
// 7-day free PROSTO-1 (or legacy Trial). Deleted instances still count.
func (s *Store) UserHasFreeWeek(ctx context.Context, userID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, nil
	}
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM vps.instances i
			JOIN vps.plans p ON p.id = i.plan_id
			WHERE i.user_id = $1::uuid
			  AND (
			    COALESCE((i.provider_meta->>'free_week')::boolean, false)
			    OR COALESCE((i.provider_meta->>'trial')::boolean, false)
			    OR LOWER(COALESCE(p.tier, '')) = 'trial'
			  )
		)
		OR EXISTS (
			SELECT 1
			FROM vps.orders o
			JOIN vps.plans p ON p.id = o.plan_id
			WHERE o.user_id = $1::uuid
			  AND LOWER(COALESCE(p.tier, '')) = 'trial'
		)
		OR EXISTS (
			SELECT 1 FROM vps.free_week_claims WHERE user_id = $1::uuid
		)
	`, userID).Scan(&exists)
	return exists, err
}

func userHasFreeWeekTx(ctx context.Context, tx pgx.Tx, userID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, nil
	}
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM vps.instances i
			JOIN vps.plans p ON p.id = i.plan_id
			WHERE i.user_id = $1::uuid
			  AND (
			    COALESCE((i.provider_meta->>'free_week')::boolean, false)
			    OR COALESCE((i.provider_meta->>'trial')::boolean, false)
			    OR LOWER(COALESCE(p.tier, '')) = 'trial'
			  )
		)
		OR EXISTS (
			SELECT 1
			FROM vps.orders o
			JOIN vps.plans p ON p.id = o.plan_id
			WHERE o.user_id = $1::uuid
			  AND LOWER(COALESCE(p.tier, '')) = 'trial'
		)
		OR EXISTS (
			SELECT 1 FROM vps.free_week_claims WHERE user_id = $1::uuid
		)
	`, userID).Scan(&exists)
	return exists, err
}

// UserHasTrialInstance is kept for callers; same as UserHasFreeWeek.
func (s *Store) UserHasTrialInstance(ctx context.Context, userID string) (bool, error) {
	return s.UserHasFreeWeek(ctx, userID)
}

func isProsto1PlanName(name string) bool {
	n := strings.ToUpper(strings.TrimSpace(name))
	return n == "PROSTO-1" || strings.HasPrefix(n, "PROSTO-1 ")
}

func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}
