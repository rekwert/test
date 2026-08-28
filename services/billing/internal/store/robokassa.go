package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateRobokassaInvoice(ctx context.Context, userID string, amount float64, description string, promoID *string, bonusAmount float64) (*Invoice, int64, error) {
	if err := s.EnsureAccount(ctx, userID); err != nil {
		return nil, 0, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx)

	var robokassaInvID int64
	if err := tx.QueryRow(ctx, `SELECT nextval('billing.robokassa_inv_seq')`).Scan(&robokassaInvID); err != nil {
		return nil, 0, err
	}

	var inv Invoice
	err = tx.QueryRow(ctx, `
		INSERT INTO billing.invoices (user_id, amount, status, description, provider, robokassa_inv_id, invoice_type, promo_id, bonus_amount)
		VALUES ($1, $2, 'pending', $3, 'robokassa', $4, 'topup', $5, $6)
		RETURNING id::text, user_id::text, amount::float8, bonus_amount::float8, invoice_type,
			instance_id::text, promo_id::text, status, provider, provider_payment_id, payment_url, description, created_at
	`, userID, amount, description, robokassaInvID, promoID, bonusAmount).Scan(
		&inv.ID, &inv.UserID, &inv.Amount, &inv.BonusAmount, &inv.InvoiceType,
		&inv.InstanceID, &inv.PromoID, &inv.Status, &inv.Provider,
		&inv.ProviderPaymentID, &inv.PaymentURL, &inv.Description, &inv.CreatedAt,
	)
	if err != nil {
		return nil, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return &inv, robokassaInvID, nil
}

func (s *Store) MarkInvoicePaidByRobokassaInvID(ctx context.Context, robokassaInvID int64, webhookAmount float64) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var invoiceID string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM billing.invoices WHERE robokassa_inv_id = $1 FOR UPDATE
	`, robokassaInvID).Scan(&invoiceID)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	credited, err := s.finalizeTopupTx(ctx, tx, invoiceID, webhookAmount)
	if err != nil {
		return false, err
	}
	var userID string
	if credited {
		_ = tx.QueryRow(ctx, `SELECT user_id::text FROM billing.invoices WHERE id = $1`, invoiceID).Scan(&userID)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	if credited {
		s.afterTopupCredit(ctx, userID)
	}
	return credited, nil
}

func (s *Store) MarkInvoiceFailedByRobokassaInvID(ctx context.Context, robokassaInvID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE billing.invoices
		SET status = 'failed', updated_at = now()
		WHERE robokassa_inv_id = $1 AND status = 'pending'
	`, robokassaInvID)
	return err
}
