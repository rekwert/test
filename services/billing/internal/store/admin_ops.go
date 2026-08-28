package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrInsufficientBalance = errors.New("insufficient balance")

func (s *Store) SumUserTopups(ctx context.Context, userID string) (float64, error) {
	var total float64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(i.amount + COALESCE(i.bonus_amount, 0))::float8, 0)
		FROM billing.invoices i
		WHERE i.user_id = $1 AND i.status = 'paid' AND i.invoice_type = 'topup'
	`, userID).Scan(&total)
	return total, err
}

func (s *Store) SetBillingStatus(ctx context.Context, userID, status string) error {
	if !validBillingStatus(status) {
		return ErrInvalidBillingStatus
	}
	if err := s.EnsureAccount(ctx, userID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE billing.accounts SET billing_status = $2, updated_at = now()
		WHERE user_id = $1
	`, userID, status)
	return err
}

func (s *Store) GetBillingStatus(ctx context.Context, userID string) (string, error) {
	if err := s.EnsureAccount(ctx, userID); err != nil {
		return "", err
	}
	var status string
	err := s.pool.QueryRow(ctx, `
		SELECT billing_status FROM billing.accounts WHERE user_id = $1
	`, userID).Scan(&status)
	return status, err
}

func (s *Store) AdminRefund(ctx context.Context, userID string, amount float64, reason, staffID string) (float64, error) {
	if amount <= 0 {
		return 0, errors.New("amount must be positive")
	}
	if err := s.EnsureAccount(ctx, userID); err != nil {
		return 0, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var balanceKopecks int64
	if err := tx.QueryRow(ctx, `
		SELECT balance_kopecks FROM billing.accounts WHERE user_id = $1 FOR UPDATE
	`, userID).Scan(&balanceKopecks); err != nil {
		return 0, err
	}
	debitKopecks := rubToKopecks(amount)
	if balanceKopecks < debitKopecks {
		return kopecksToRub(balanceKopecks), ErrInsufficientBalance
	}
	newKopecks := balanceKopecks - debitKopecks
	newBalance := kopecksToRub(newKopecks)

	if _, err := tx.Exec(ctx, `
		UPDATE billing.accounts SET balance_kopecks = $2, updated_at = now()
		WHERE user_id = $1
	`, userID, newKopecks); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.adjustments (user_id, amount, kind, reason, staff_id, balance_after)
		VALUES ($1, $2, 'refund', $3, NULLIF($4, '')::uuid, $5)
	`, userID, amount, reason, staffID, newBalance); err != nil {
		return 0, err
	}

	clawReason := reason
	if clawReason == "" {
		clawReason = "Referral clawback after customer refund"
	}
	if err := s.ClawbackReferralForUser(ctx, tx, userID, clawReason, staffID); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return newBalance, nil
}

func (s *Store) AdminCredit(ctx context.Context, userID string, amount float64, reason, staffID string) (float64, error) {
	if amount <= 0 {
		return 0, errors.New("amount must be positive")
	}
	if err := s.EnsureAccount(ctx, userID); err != nil {
		return 0, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var balanceKopecks int64
	if err := tx.QueryRow(ctx, `
		SELECT balance_kopecks FROM billing.accounts WHERE user_id = $1 FOR UPDATE
	`, userID).Scan(&balanceKopecks); err != nil {
		return 0, err
	}
	creditKopecks := rubToKopecks(amount)
	newKopecks := balanceKopecks + creditKopecks
	newBalance := kopecksToRub(newKopecks)

	if _, err := tx.Exec(ctx, `
		UPDATE billing.accounts SET balance_kopecks = $2, updated_at = now()
		WHERE user_id = $1
	`, userID, newKopecks); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.adjustments (user_id, amount, kind, reason, staff_id, balance_after)
		VALUES ($1, $2, 'credit', $3, NULLIF($4, '')::uuid, $5)
	`, userID, amount, reason, staffID, newBalance); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	s.afterTopupCredit(ctx, userID)
	return newBalance, nil
}

func (s *Store) TotalRevenue(ctx context.Context) (float64, error) {
	var total float64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount)::float8, 0) FROM billing.invoices WHERE status = 'paid' AND invoice_type = 'topup'
	`).Scan(&total)
	return total, err
}

func (s *Store) ListAdjustments(ctx context.Context, userID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, amount::float8, kind, COALESCE(reason, ''), staff_id::text,
		       balance_after::float8, created_at
		FROM billing.adjustments
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []map[string]any
	for rows.Next() {
		var id, kind, reason string
		var amount float64
		var staffID *string
		var balanceAfter *float64
		var createdAt any
		if err := rows.Scan(&id, &amount, &kind, &reason, &staffID, &balanceAfter, &createdAt); err != nil {
			return nil, err
		}
		row := map[string]any{
			"id":         id,
			"amount":     amount,
			"kind":       kind,
			"reason":     reason,
			"created_at": createdAt,
		}
		if staffID != nil {
			row["staff_id"] = *staffID
		}
		if balanceAfter != nil {
			row["balance_after"] = *balanceAfter
		}
		items = append(items, row)
	}
	return items, rows.Err()
}

func (s *Store) GetBalanceWithStatus(ctx context.Context, userID string) (*Account, string, error) {
	if err := s.EnsureAccount(ctx, userID); err != nil {
		return nil, "", err
	}
	var acc Account
	var status string
	var balanceKopecks int64
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, balance_kopecks, currency, billing_status
		FROM billing.accounts
		WHERE user_id = $1
	`, userID).Scan(&acc.UserID, &balanceKopecks, &acc.Currency, &status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, "", err
		}
		return nil, "", err
	}
	acc.Balance = kopecksToRub(balanceKopecks)
	return &acc, status, nil
}
