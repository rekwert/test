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
