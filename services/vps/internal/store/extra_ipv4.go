package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// SetInstanceAllIPs stores the full IPv4 list in provider_meta for the LK credentials view.
func (s *Store) SetInstanceAllIPs(ctx context.Context, instanceID string, ips []string) error {
	if len(ips) == 0 {
		return fmt.Errorf("ips required")
	}
	raw, err := json.Marshal(ips)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		    || jsonb_build_object('all_ips', $2::jsonb),
		    updated_at = now()
		WHERE id = $1::uuid AND state <> 'deleted'
	`, instanceID, string(raw))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInstanceNotFound
	}
	return nil
}

// ChargeExtraIPv4Fee debits the prepaid charge for additional IPv4 addresses.
func (s *Store) ChargeExtraIPv4Fee(ctx context.Context, userID, instanceID string, amount float64, qty int) error {
	if amount <= 0 || qty <= 0 {
		return fmt.Errorf("invalid extra ipv4 charge")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.accounts (user_id) VALUES ($1::uuid) ON CONFLICT DO NOTHING
	`, userID); err != nil {
		return err
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
		return err
	}
	if billingStatus == "suspended" {
		return ErrBillingSuspended
	}
	if balance < amount {
		return ErrInsufficientBalance
	}

	desc := fmt.Sprintf("VPS extra IPv4 x%d", qty)
	var balanceAfter float64
	if err := tx.QueryRow(ctx, `
		UPDATE billing.accounts
		SET balance = balance - $2, updated_at = now()
		WHERE user_id = $1::uuid
		RETURNING balance::float8
	`, userID, amount).Scan(&balanceAfter); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, instance_id, balance_after)
		VALUES ($1::uuid, $2, 'paid', $3, 'balance', 'charge', $4::uuid, $5)
	`, userID, amount, desc, instanceID, balanceAfter); err != nil {
		return err
	}

	if err := s.processReferralPayment(ctx, tx, userID, amount); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RefundExtraIPv4Fee credits the charge back after a hypervisor failure.
func (s *Store) RefundExtraIPv4Fee(ctx context.Context, userID, instanceID string, amount float64, qty int, reason string) error {
	if amount <= 0 {
		return nil
	}
	desc := reason
	if desc == "" {
		desc = fmt.Sprintf("VPS extra IPv4 refund x%d (instance %s)", qty, instanceID)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM billing.adjustments
			WHERE user_id = $1::uuid AND reason = $2
		)
	`, userID, desc).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

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
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.adjustments (user_id, amount, kind, reason)
		VALUES ($1::uuid, $2, 'credit', $3)
	`, userID, amount, desc); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
