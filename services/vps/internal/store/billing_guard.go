package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// ClientPowerBlocked reports whether the client must not start/reboot (suspended for non-payment).
func ClientPowerBlocked(billingStatus string) bool {
	switch billingStatus {
	case "suspended", "cancelled":
		return true
	default:
		return false
	}
}

// ClientBillingAccessBlocked is true when instance or account billing forbids client access
// (credentials, console, power-on).
func (s *Store) ClientBillingAccessBlocked(ctx context.Context, instanceID string) (bool, error) {
	var instStatus, acctStatus string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(i.billing_status, 'active'), COALESCE(a.billing_status, 'active')
		FROM vps.instances i
		JOIN billing.accounts a ON a.user_id = i.user_id
		WHERE i.id = $1::uuid
	`, instanceID).Scan(&instStatus, &acctStatus)
	if err == pgx.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return ClientPowerBlocked(instStatus) || ClientPowerBlocked(acctStatus), nil
}

// BillingAllowsPowerOn is true only when account and instance billing are active (paid up).
func (s *Store) BillingAllowsPowerOn(ctx context.Context, instanceID string) (bool, error) {
	var instStatus, acctStatus string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(i.billing_status, 'active'), COALESCE(a.billing_status, 'active')
		FROM vps.instances i
		JOIN billing.accounts a ON a.user_id = i.user_id
		WHERE i.id = $1::uuid
	`, instanceID).Scan(&instStatus, &acctStatus)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return instStatus == "active" && acctStatus == "active", nil
}
