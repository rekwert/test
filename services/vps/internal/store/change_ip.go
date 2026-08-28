package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrChangeIPInProgress = errors.New("change ip already in progress")

const ChangeIPFee = 150.0

const changeIPLockStaleAfter = "15 minutes"

// TryBeginChangeIP acquires a per-instance lock to prevent concurrent IP changes.
func (s *Store) TryBeginChangeIP(ctx context.Context, userID, instanceID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb)
		    || jsonb_build_object('change_ip_in_progress', to_jsonb(now())),
		    updated_at = updated_at
		WHERE id = $1::uuid AND user_id = $2::uuid AND state = 'running'
		  AND (
		    provider_meta->>'change_ip_in_progress' IS NULL
		    OR (provider_meta->>'change_ip_in_progress')::timestamptz < now() - '`+changeIPLockStaleAfter+`'::interval
		  )
	`, instanceID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ClearChangeIPLock(ctx context.Context, instanceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET provider_meta = COALESCE(provider_meta, '{}'::jsonb) - 'change_ip_in_progress'
		WHERE id = $1::uuid
	`, instanceID)
	return err
}

// ChargeChangeIPFee debits the fixed IP change fee from the user balance.
func (s *Store) ChargeChangeIPFee(ctx context.Context, userID, instanceID string) error {
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
	if balance < ChangeIPFee {
		return ErrInsufficientBalance
	}

	var balanceAfter float64
	if err := tx.QueryRow(ctx, `
		UPDATE billing.accounts
		SET balance = balance - $2, updated_at = now()
		WHERE user_id = $1::uuid
		RETURNING balance::float8
	`, userID, ChangeIPFee).Scan(&balanceAfter); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.invoices (user_id, amount, status, description, provider, invoice_type, instance_id, balance_after)
		VALUES ($1::uuid, $2, 'paid', $3, 'balance', 'charge', $4::uuid, $5)
	`, userID, ChangeIPFee, "VPS IP change fee", instanceID, balanceAfter); err != nil {
		return err
	}

	if err := s.processReferralPayment(ctx, tx, userID, ChangeIPFee); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RefundChangeIPFee credits the IP change fee back (best-effort after HV failure).
func (s *Store) RefundChangeIPFee(ctx context.Context, userID, instanceID, reason string) error {
	desc := reason
	if desc == "" {
		desc = fmt.Sprintf("VPS IP change refund (instance %s)", instanceID)
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
	`, userID, ChangeIPFee); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.adjustments (user_id, amount, kind, reason)
		VALUES ($1::uuid, $2, 'credit', $3)
	`, userID, ChangeIPFee, desc); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetInstanceIPAddress forcibly updates the instance public IP.
func (s *Store) SetInstanceIPAddress(ctx context.Context, instanceID, ip string, logOpts *IPAssignmentLogOpts) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return fmt.Errorf("ip required")
	}
	var userID string
	var oldIP *string
	err := s.pool.QueryRow(ctx, `
		SELECT user_id::text, host(ip_address)
		FROM vps.instances
		WHERE id = $1::uuid AND state <> 'deleted'
	`, instanceID).Scan(&userID, &oldIP)
	if err != nil {
		return ErrInstanceNotFound
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET ip_address = $2::inet, updated_at = now()
		WHERE id = $1::uuid AND state <> 'deleted'
	`, instanceID, ip)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInstanceNotFound
	}
	if logOpts != nil {
		prev := ""
		if oldIP != nil {
			prev = strings.TrimSpace(*oldIP)
		}
		if prev != ip {
			_ = s.LogIPChange(ctx, userID, instanceID, prev, ip, logOpts)
		}
	}
	return nil
}
