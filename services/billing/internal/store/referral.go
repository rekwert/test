package store

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	referrerPercent      = 10
	referralHoldDuration = 30 * 24 * time.Hour
)

func (s *Store) processReferralPayment(ctx context.Context, tx pgx.Tx, userID string, amount float64) error {
	return s.processReferralPaymentKind(ctx, tx, userID, amount, "payment", "")
}

func (s *Store) processReferralPaymentKind(ctx context.Context, tx pgx.Tx, userID string, amount float64, kind, ref string) error {
	var regID, referrerID string

	err := tx.QueryRow(ctx, `
		SELECT id::text, referrer_user_id::text
		FROM referral.registrations
		WHERE referred_user_id = $1
		FOR UPDATE
	`, userID).Scan(&regID, &referrerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	commission := math.Round(amount*float64(referrerPercent)) / 100
	if commission <= 0 {
		return nil
	}

	var prior int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM referral.earnings
		WHERE registration_id = $1::uuid
		  AND status NOT IN ('cancelled', 'clawed_back')
	`, regID).Scan(&prior); err != nil {
		return err
	}
	if prior > 0 {
		return nil
	}

	availableAt := time.Now().UTC().Add(referralHoldDuration)
	_, err = tx.Exec(ctx, `
		INSERT INTO referral.earnings (
			referrer_user_id, registration_id, amount, status, available_at,
			source_user_id, source_kind, source_ref
		)
		VALUES ($1, $2, $3, 'pending', $4, $5::uuid, $6, $7)
	`, referrerID, regID, commission, availableAt, userID, kind, ref)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE referral.registrations
		SET status = CASE WHEN status = 'registered' THEN 'paid' ELSE status END,
		    updated_at = now()
		WHERE id = $1
	`, regID)
	return err
}

// SettleDueReferralEarnings credits referrer balances for pending earnings past the hold.
func (s *Store) SettleDueReferralEarnings(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id::text, referrer_user_id::text, registration_id::text, amount::float8
		FROM referral.earnings
		WHERE status = 'pending' AND available_at <= now()
		ORDER BY available_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type item struct {
		id, referrerID, regID string
		amount                float64
	}
	items := make([]item, 0)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.referrerID, &it.regID, &it.amount); err != nil {
			return 0, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	rows.Close()

	settled := 0
	for _, it := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO billing.accounts (user_id) VALUES ($1) ON CONFLICT DO NOTHING
		`, it.referrerID); err != nil {
			return settled, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE billing.accounts
			SET balance = balance + $2, updated_at = now()
			WHERE user_id = $1
		`, it.referrerID, it.amount); err != nil {
			return settled, err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE referral.earnings
			SET status = 'credited', credited_at = now()
			WHERE id = $1::uuid AND status = 'pending'
		`, it.id)
		if err != nil {
			return settled, err
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE referral.registrations
			SET total_earned = total_earned + $2, status = 'earning', updated_at = now()
			WHERE id = $1::uuid
		`, it.regID, it.amount); err != nil {
			return settled, err
		}
		settled++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return settled, nil
}

// ClawbackReferralForUser cancels pending and reverses credited earnings for a referred user.
func (s *Store) ClawbackReferralForUser(ctx context.Context, tx pgx.Tx, referredUserID, reason, staffID string) error {
	rows, err := tx.Query(ctx, `
		SELECT e.id::text, e.referrer_user_id::text, e.registration_id::text, e.amount::float8, e.status
		FROM referral.earnings e
		WHERE e.status IN ('pending', 'credited')
		  AND e.registration_id IN (
			SELECT id FROM referral.registrations WHERE referred_user_id = $1::uuid
		  )
		ORDER BY e.created_at ASC
		FOR UPDATE
	`, referredUserID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type item struct {
		id, referrerID, regID, status string
		amount                        float64
	}
	items := make([]item, 0)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.referrerID, &it.regID, &it.amount, &it.status); err != nil {
			return err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	if reason == "" {
		reason = "Referral clawback after refund"
	}

	for _, it := range items {
		if it.status == "pending" {
			if _, err := tx.Exec(ctx, `
				UPDATE referral.earnings
				SET status = 'cancelled', clawed_at = now()
				WHERE id = $1::uuid AND status = 'pending'
			`, it.id); err != nil {
				return err
			}
			continue
		}

		// credited → claw back from referrer balance (floor at 0)
		var bal float64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(balance, 0)::float8 FROM billing.accounts WHERE user_id = $1 FOR UPDATE
		`, it.referrerID).Scan(&bal); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		debit := it.amount
		if bal < debit {
			debit = bal
		}
		if debit > 0 {
			if _, err := tx.Exec(ctx, `
				UPDATE billing.accounts
				SET balance = balance - $2, updated_at = now()
				WHERE user_id = $1
			`, it.referrerID, debit); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO billing.adjustments (user_id, amount, kind, reason, staff_id)
				VALUES ($1, $2, 'referral_clawback', $3, NULLIF($4, '')::uuid)
			`, it.referrerID, debit, reason, staffID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE referral.earnings
			SET status = 'clawed_back', clawed_at = now()
			WHERE id = $1::uuid AND status = 'credited'
		`, it.id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE referral.registrations
			SET total_earned = GREATEST(0, total_earned - $2), updated_at = now()
			WHERE id = $1::uuid
		`, it.regID, debit); err != nil {
			return err
		}
	}
	return nil
}
