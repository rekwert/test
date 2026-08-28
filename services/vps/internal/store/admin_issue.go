package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
)

var (
	ErrInvalidIssueDays = errors.New("days must be at least 1")
)

const MaxAdminIssueDays = 9999

type AdminIssueInput struct {
	UserID       string
	StaffID      string
	PlanID       string
	Region       string
	Days         int
	Hostname     string
	RootPassword string
	OSTemplateID      string
	SoftwareProfileID string
}

// CreateAdminIssuedOrder provisions a VPS for a client without charging balance.
// Prepaid period is the given number of days; auto-renew is off.
func (s *Store) CreateAdminIssuedOrder(ctx context.Context, in AdminIssueInput) (*CreateOrderResult, error) {
	in.UserID = strings.TrimSpace(in.UserID)
	in.PlanID = strings.TrimSpace(in.PlanID)
	in.Region = strings.TrimSpace(strings.ToLower(in.Region))
	in.Hostname = strings.TrimSpace(strings.ToLower(in.Hostname))
	in.RootPassword = strings.TrimSpace(in.RootPassword)
	in.OSTemplateID = strings.TrimSpace(in.OSTemplateID)
	in.SoftwareProfileID = strings.TrimSpace(in.SoftwareProfileID)
	in.StaffID = strings.TrimSpace(in.StaffID)

	if in.UserID == "" || in.PlanID == "" || in.Region == "" {
		return nil, errors.New("user_id, plan_id and region required")
	}
	if in.Days < 1 || in.Days > MaxAdminIssueDays {
		return nil, ErrInvalidIssueDays
	}

	tier, err := s.ResolvePlanTier(ctx, in.PlanID)
	if err != nil {
		return nil, err
	}
	if tier == "" {
		return nil, ErrPlanNotFound
	}

	if in.OSTemplateID == "" {
		templates, err := s.ListActiveOSTemplates(ctx)
		if err != nil {
			return nil, err
		}
		for _, t := range templates {
			fam := strings.ToLower(t.Family)
			if fam == "debian" || fam == "ubuntu" || strings.Contains(strings.ToLower(t.ID), "ubuntu") {
				in.OSTemplateID = t.ID
				break
			}
		}
		if in.OSTemplateID == "" && len(templates) > 0 {
			in.OSTemplateID = templates[0].ID
		}
		if in.OSTemplateID == "" {
			return nil, errors.New("no active os template")
		}
	}

	softwareID := in.SoftwareProfileID
	if softwareID == "" {
		softwareID = "clean"
	}
	catalogPlanName := ""
	if p, ok := catalog.PlanByID(in.PlanID); ok {
		catalogPlanName = p.Name
	}
	if !catalog.SoftwareAllowedForPlan(catalogPlanName, tier, in.OSTemplateID, softwareID) {
		return nil, errors.New("software profile not available for os")
	}

	nodeID, waitlisted, err := s.resolveOrderNode(ctx, in.Region, tier)
	if err != nil {
		return nil, err
	}

	sshKeys, err := s.ListSSHPublicKeys(ctx, in.UserID, nil)
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
	var planRegion string
	err = tx.QueryRow(ctx, `
		SELECT name, COALESCE(region, '')
		FROM vps.plans
		WHERE id = $1::uuid AND active = true
		  AND COALESCE(product_type, 'vps') <> 'dedicated'
	`, in.PlanID).Scan(&planName, &planRegion)
	if err == pgx.ErrNoRows {
		return nil, ErrPlanNotFound
	}
	if err != nil {
		return nil, err
	}
	if planRegion != "" && planRegion != in.Region {
		return nil, errors.New("plan not available in region")
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.accounts (user_id) VALUES ($1::uuid) ON CONFLICT DO NOTHING
	`, in.UserID); err != nil {
		return nil, err
	}

	var billingStatus string
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(a.billing_status, 'active')
		FROM billing.accounts a
		WHERE a.user_id = $1::uuid
		FOR UPDATE
	`, in.UserID).Scan(&billingStatus)
	if err != nil {
		return nil, err
	}
	if billingStatus == "suspended" {
		return nil, ErrBillingSuspended
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
	prepaidDays := in.Days

	var orderNumber int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO vps.orders (id, user_id, plan_id, region, status, os_template_id, software_profile_id, hostname)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'paid', $5, $6, $7)
		RETURNING order_number
	`, orderID, in.UserID, in.PlanID, in.Region, in.OSTemplateID, softwareID, hostArg).Scan(&orderNumber); err != nil {
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
			$8, $9, 30, NULL, $10::jsonb,
			false, jsonb_build_object(
				'initial_prepaid_days', $11::int,
				'admin_issued', true,
				'no_renew', true,
				'trial', false,
				'free_week', false
			)
		)
	`, instanceID, in.UserID, orderID, in.PlanID, in.Region, nodeID, instanceState, hostArg, rootPassArg, sshKeysJSON, prepaidDays); err != nil {
		return nil, err
	}

	hostLabel := in.Hostname
	if hostLabel == "" {
		hostLabel = planName
	}
	desc := fmt.Sprintf("VPS · %s · admin · %dd", in.Region, prepaidDays)
	_ = hostLabel

	var balanceAfter float64
	_ = tx.QueryRow(ctx, `
		SELECT COALESCE(balance, 0)::float8 FROM billing.accounts WHERE user_id = $1::uuid
	`, in.UserID).Scan(&balanceAfter)

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, instance_id, balance_after)
		VALUES ($1::uuid, 0, 'paid', $2, 'admin', 'charge', $3::uuid, $4)
	`, in.UserID, desc, instanceID, balanceAfter); err != nil {
		return nil, err
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
			"software_profile_id": softwareID,
			"ssh_keys":            sshKeys,
		})
		if _, err := tx.Exec(ctx, `
			INSERT INTO vps.outbox (event_type, payload)
			VALUES ('instance.provision_requested', $1::jsonb)
		`, outboxPayload); err != nil {
			return nil, err
		}
	}

	details, _ := json.Marshal(map[string]any{
		"plan_id":     in.PlanID,
		"region":      in.Region,
		"days":        prepaidDays,
		"instance_id": instanceID,
		"order_id":    orderID,
		"queued":      waitlisted,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO vps.admin_actions (staff_id, user_id, instance_id, action, details)
		VALUES (NULLIF($1, '')::uuid, $2::uuid, $3::uuid, 'admin.issue_instance', $4::jsonb)
	`, in.StaffID, in.UserID, instanceID, details); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &CreateOrderResult{
		OrderID:     orderID,
		OrderNumber: orderNumber,
		InstanceID:  instanceID,
		Amount:      0,
		Status:      "paid",
		Queued:      waitlisted,
	}, nil
}
