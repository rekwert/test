package store

import (
	"context"
	"errors"
	"log"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
)

var (
	ErrPromoNotFound    = errors.New("promo not found")
	ErrPromoInvalid     = errors.New("promo invalid")
	ErrPromoExhausted   = errors.New("promo exhausted")
	ErrPromoAlreadyUsed = errors.New("promo already used")
)

type PromoPreview struct {
	PromoID         string
	Code            string
	Kind            string
	Description     string
	BonusAmount     float64
	DiscountPercent float64
}

type PromoCode struct {
	ID              string
	Code            string
	Kind            string
	Value           float64
	MinAmount       float64
	MaxRedemptions  *int
	RedemptionCount int
	PerUserLimit    int
	Description     *string
}

func (s *Store) finalizeTopupTx(ctx context.Context, tx pgx.Tx, invoiceID string, webhookAmount float64) (bool, error) {
	var userID, status, invoiceType string
	var invoiceAmount, bonusAmount float64
	var promoID *string
	err := tx.QueryRow(ctx, `
		SELECT user_id::text, status, invoice_type, amount::float8, bonus_amount::float8, promo_id::text
		FROM billing.invoices
		WHERE id = $1
		FOR UPDATE
	`, invoiceID).Scan(&userID, &status, &invoiceType, &invoiceAmount, &bonusAmount, &promoID)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if status == "paid" {
		return false, nil
	}
	if invoiceType != "topup" {
		return false, errors.New("not a topup invoice")
	}

	paidAmount := invoiceAmount
	if webhookAmount > 0 && math.Abs(webhookAmount-invoiceAmount) > 0.01 {
		log.Printf("billing: webhook amount mismatch invoice=%s webhook=%.2f invoice=%.2f — using invoice amount",
			invoiceID, webhookAmount, invoiceAmount)
	}
	totalCredit := paidAmount + bonusAmount

	if promoID != nil && *promoID != "" {
		if err := s.recordPromoRedemptionTx(ctx, tx, *promoID, userID, invoiceID, bonusAmount, 0); err != nil {
			if errors.Is(err, ErrPromoExhausted) || errors.Is(err, ErrPromoAlreadyUsed) {
				log.Printf("billing: promo redemption skipped invoice=%s: %v", invoiceID, err)
				totalCredit = paidAmount
			} else {
				return false, err
			}
		}
	}

	var balanceAfter float64
	if err := tx.QueryRow(ctx, `
		UPDATE billing.accounts
		SET balance = balance + $2, updated_at = now()
		WHERE user_id = $1
		RETURNING balance::float8
	`, userID, totalCredit).Scan(&balanceAfter); err != nil {
		return false, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE billing.invoices
		SET status = 'paid', balance_after = $2, updated_at = now()
		WHERE id = $1
	`, invoiceID, balanceAfter); err != nil {
		return false, err
	}

	if err := s.processReferralPayment(ctx, tx, userID, paidAmount); err != nil {
		return false, err
	}

	return true, nil
}

func (s *Store) getPromoByCode(ctx context.Context, code string) (*PromoCode, error) {
	var p PromoCode
	var desc *string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, code, kind, value::float8, min_amount::float8,
			max_redemptions, redemption_count, per_user_limit, description
		FROM billing.promo_codes
		WHERE lower(code) = lower($1) AND active = true
	`, code).Scan(
		&p.ID, &p.Code, &p.Kind, &p.Value, &p.MinAmount,
		&p.MaxRedemptions, &p.RedemptionCount, &p.PerUserLimit, &desc,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrPromoNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Description = desc
	return &p, nil
}

func (s *Store) validatePromoWindow(ctx context.Context, p *PromoCode) error {
	var valid bool
	err := s.pool.QueryRow(ctx, `
		SELECT (
			(valid_from IS NULL OR valid_from <= now()) AND
			(valid_until IS NULL OR valid_until >= now()) AND
			(max_redemptions IS NULL OR redemption_count < max_redemptions)
		)
		FROM billing.promo_codes WHERE id = $1::uuid
	`, p.ID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrPromoInvalid
	}
	return nil
}

func (s *Store) userPromoUses(ctx context.Context, promoID, userID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM billing.promo_redemptions
		WHERE promo_id = $1::uuid AND user_id = $2::uuid
	`, promoID, userID).Scan(&count)
	return count, err
}

func (s *Store) PreviewPromo(ctx context.Context, userID, code, contextKind string, amount float64) (*PromoPreview, error) {
	p, err := s.getPromoByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if err := s.validatePromoWindow(ctx, p); err != nil {
		return nil, err
	}
	uses, err := s.userPromoUses(ctx, p.ID, userID)
	if err != nil {
		return nil, err
	}
	if uses >= p.PerUserLimit {
		return nil, ErrPromoAlreadyUsed
	}

	preview := &PromoPreview{
		PromoID: p.ID,
		Code:    p.Code,
		Kind:    p.Kind,
	}
	if p.Description != nil {
		preview.Description = *p.Description
	}

	switch p.Kind {
	case "credit":
		if contextKind != "apply" {
			return nil, ErrPromoInvalid
		}
		preview.BonusAmount = p.Value
	case "topup_bonus_percent":
		if amount < p.MinAmount {
			return nil, ErrPromoInvalid
		}
		preview.BonusAmount = math.Round(amount*p.Value) / 100
	case "topup_bonus_fixed":
		if amount < p.MinAmount {
			return nil, ErrPromoInvalid
		}
		preview.BonusAmount = p.Value
	case "charge_discount_percent":
		preview.DiscountPercent = p.Value
	default:
		return nil, ErrPromoInvalid
	}

	return preview, nil
}

func (s *Store) ApplyCreditPromo(ctx context.Context, userID, code string) (float64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	preview, err := s.previewPromoLocked(ctx, tx, userID, code, "apply", 0)
	if err != nil {
		return 0, err
	}
	if preview.Kind != "credit" {
		return 0, ErrPromoInvalid
	}

	if err := s.EnsureAccountTx(ctx, tx, userID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE billing.accounts SET balance = balance + $2, updated_at = now()
		WHERE user_id = $1
	`, userID, preview.BonusAmount); err != nil {
		return 0, err
	}

	if err := s.recordPromoRedemptionTx(ctx, tx, preview.PromoID, userID, "", preview.BonusAmount, 0); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return preview.BonusAmount, nil
}

func (s *Store) ResolveTopupPromo(ctx context.Context, userID, code string, amount float64) (*PromoPreview, error) {
	if code == "" {
		return nil, nil
	}
	preview, err := s.PreviewPromo(ctx, userID, code, "topup", amount)
	if err != nil {
		return nil, err
	}
	if preview.Kind != "topup_bonus_percent" && preview.Kind != "topup_bonus_fixed" {
		return nil, ErrPromoInvalid
	}
	return preview, nil
}

func (s *Store) recordPromoRedemptionTx(ctx context.Context, tx pgx.Tx, promoID, userID, invoiceID string, bonus, discount float64) error {
	var perUserLimit int
	var valid bool
	err := tx.QueryRow(ctx, `
		SELECT per_user_limit,
			(
				(valid_from IS NULL OR valid_from <= now()) AND
				(valid_until IS NULL OR valid_until >= now()) AND
				(max_redemptions IS NULL OR redemption_count < max_redemptions)
			)
		FROM billing.promo_codes
		WHERE id = $1::uuid
		FOR UPDATE
	`, promoID).Scan(&perUserLimit, &valid)
	if err == pgx.ErrNoRows {
		return ErrPromoNotFound
	}
	if err != nil {
		return err
	}
	if !valid {
		return ErrPromoExhausted
	}
	var uses int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM billing.promo_redemptions
		WHERE promo_id = $1::uuid AND user_id = $2::uuid
	`, promoID, userID).Scan(&uses); err != nil {
		return err
	}
	if uses >= perUserLimit {
		return ErrPromoAlreadyUsed
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.promo_redemptions (promo_id, user_id, invoice_id, bonus_amount, discount_amount)
		VALUES ($1::uuid, $2::uuid, NULLIF($3, '')::uuid, $4, $5)
	`, promoID, userID, invoiceID, bonus, discount); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE billing.promo_codes
		SET redemption_count = redemption_count + 1
		WHERE id = $1::uuid
	`, promoID)
	return err
}

func (s *Store) previewPromoLocked(ctx context.Context, tx pgx.Tx, userID, code, contextKind string, amount float64) (*PromoPreview, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, ErrPromoNotFound
	}

	var p PromoCode
	var desc *string
	err := tx.QueryRow(ctx, `
		SELECT id::text, code, kind, value::float8, min_amount::float8,
			max_redemptions, redemption_count, per_user_limit, description
		FROM billing.promo_codes
		WHERE lower(code) = lower($1) AND active = true
		FOR UPDATE
	`, code).Scan(
		&p.ID, &p.Code, &p.Kind, &p.Value, &p.MinAmount,
		&p.MaxRedemptions, &p.RedemptionCount, &p.PerUserLimit, &desc,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrPromoNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Description = desc

	var valid bool
	if err := tx.QueryRow(ctx, `
		SELECT (
			(valid_from IS NULL OR valid_from <= now()) AND
			(valid_until IS NULL OR valid_until >= now()) AND
			(max_redemptions IS NULL OR redemption_count < max_redemptions)
		)
		FROM billing.promo_codes WHERE id = $1::uuid
	`, p.ID).Scan(&valid); err != nil {
		return nil, err
	}
	if !valid {
		return nil, ErrPromoInvalid
	}

	var uses int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM billing.promo_redemptions
		WHERE promo_id = $1::uuid AND user_id = $2::uuid
	`, p.ID, userID).Scan(&uses); err != nil {
		return nil, err
	}
	if uses >= p.PerUserLimit {
		return nil, ErrPromoAlreadyUsed
	}

	preview := &PromoPreview{
		PromoID: p.ID,
		Code:    p.Code,
		Kind:    p.Kind,
	}
	if p.Description != nil {
		preview.Description = *p.Description
	}

	switch p.Kind {
	case "credit":
		if contextKind != "apply" {
			return nil, ErrPromoInvalid
		}
		preview.BonusAmount = p.Value
	case "topup_bonus_percent":
		if amount < p.MinAmount {
			return nil, ErrPromoInvalid
		}
		preview.BonusAmount = math.Round(amount*p.Value) / 100
	case "topup_bonus_fixed":
		if amount < p.MinAmount {
			return nil, ErrPromoInvalid
		}
		preview.BonusAmount = p.Value
	case "charge_discount_percent":
		preview.DiscountPercent = p.Value
	default:
		return nil, ErrPromoInvalid
	}

	return preview, nil
}

func (s *Store) EnsureAccountTx(ctx context.Context, tx pgx.Tx, userID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO billing.accounts (user_id) VALUES ($1) ON CONFLICT DO NOTHING
	`, userID)
	return err
}

func (s *Store) ApplyChargeDiscountPromo(ctx context.Context, userID, code string, instanceID *string) (*PromoPreview, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	preview, err := s.previewPromoLocked(ctx, tx, userID, code, "charge", 0)
	if err != nil {
		return nil, err
	}
	if preview.Kind != "charge_discount_percent" {
		return nil, ErrPromoInvalid
	}

	if err := s.recordPromoRedemptionTx(ctx, tx, preview.PromoID, userID, "", 0, preview.DiscountPercent); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO billing.promo_entitlements (user_id, promo_id, discount_percent, instance_id)
		VALUES ($1, $2::uuid, $3, $4::uuid)
	`, userID, preview.PromoID, preview.DiscountPercent, instanceID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return preview, nil
}

func (s *Store) bestChargeDiscount(ctx context.Context, userID, instanceID string) (float64, error) {
	var discount float64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(discount_percent), 0)::float8
		FROM billing.promo_entitlements
		WHERE user_id = $1::uuid AND active = true
		AND (expires_at IS NULL OR expires_at > now())
		AND (instance_id IS NULL OR instance_id = $2::uuid)
	`, userID, instanceID).Scan(&discount)
	return discount, err
}
