package store

import (
	"context"
	"math"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/dbpool"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/platformmigrate"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Account struct {
	UserID   string
	Balance  float64
	Currency string
}

type Invoice struct {
	ID                string
	UserID            string
	Amount            float64
	BonusAmount       float64
	InvoiceType       string
	InstanceID        *string
	PromoID           *string
	Status            string
	Provider          *string
	ProviderPaymentID *string
	PaymentURL        *string
	Description       *string
	BalanceAfter      *float64
	CreatedAt         time.Time
	// Enriched from instance/plan when available (ListInvoices).
	Region      string
	PlanTier    string
	PlanName    string
	ProductType string
}

type Store struct {
	pool *pgxpool.Pool
}

func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := dbpool.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	return platformmigrate.Apply(ctx, s.pool, "billing", migrations.FS)
}

func (s *Store) EnsureAccount(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO billing.accounts (user_id)
		VALUES ($1)
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	return err
}

func (s *Store) GetBalance(ctx context.Context, userID string) (*Account, error) {
	if err := s.EnsureAccount(ctx, userID); err != nil {
		return nil, err
	}
	var acc Account
	var balanceKopecks int64
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, balance_kopecks, currency
		FROM billing.accounts
		WHERE user_id = $1
	`, userID).Scan(&acc.UserID, &balanceKopecks, &acc.Currency)
	if err != nil {
		return nil, err
	}
	acc.Balance = float64(balanceKopecks) / 100
	return &acc, nil
}

func (s *Store) CreateTopupInvoice(ctx context.Context, userID string, amount float64, description, provider string, promoID *string, bonusAmount float64) (*Invoice, error) {
	if err := s.EnsureAccount(ctx, userID); err != nil {
		return nil, err
	}
	if provider == "" {
		provider = "tbank"
	}
	amountKopecks := int64(math.Round(amount * 100))
	bonusKopecks := int64(math.Round(bonusAmount * 100))
	var inv Invoice
	err := s.pool.QueryRow(ctx, `
		INSERT INTO billing.invoices (user_id, amount, amount_kopecks, status, description, provider, invoice_type, promo_id, bonus_amount, bonus_amount_kopecks)
		VALUES ($1, $2, $7, 'pending', $3, $4, 'topup', $5, $6, $8)
		RETURNING id::text, user_id::text, amount::float8, bonus_amount::float8, invoice_type,
			instance_id::text, promo_id::text, status, provider, provider_payment_id, payment_url, description, created_at
	`, userID, amount, description, provider, promoID, bonusAmount, amountKopecks, bonusKopecks).Scan(
		&inv.ID, &inv.UserID, &inv.Amount, &inv.BonusAmount, &inv.InvoiceType,
		&inv.InstanceID, &inv.PromoID, &inv.Status, &inv.Provider,
		&inv.ProviderPaymentID, &inv.PaymentURL, &inv.Description, &inv.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (s *Store) AttachPayment(ctx context.Context, invoiceID, paymentID, paymentURL string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE billing.invoices
		SET provider_payment_id = $2, payment_url = $3, updated_at = now()
		WHERE id = $1
	`, invoiceID, paymentID, paymentURL)
	return err
}

func (s *Store) ListInvoices(ctx context.Context, userID string) ([]Invoice, error) {
	items, _, err := s.ListInvoicesPage(ctx, userID, 50, 0)
	return items, err
}

func (s *Store) ListInvoicesPage(ctx context.Context, userID string, limit, offset int) ([]Invoice, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM billing.invoices WHERE user_id = $1
	`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text, i.user_id::text, i.amount::float8, i.bonus_amount::float8, i.invoice_type,
			i.instance_id::text, i.promo_id::text, i.status, i.provider, i.provider_payment_id, i.payment_url,
			i.description, i.balance_after::float8, i.created_at,
			COALESCE(inst.region, ''),
			COALESCE(NULLIF(lower(p.tier), ''), ''),
			COALESCE(p.name, ''),
			COALESCE(NULLIF(inst.product_type, ''), 'vps')
		FROM billing.invoices i
		LEFT JOIN vps.instances inst ON inst.id = i.instance_id
		LEFT JOIN vps.plans p ON p.id = inst.plan_id
		WHERE i.user_id = $1
		ORDER BY i.created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []Invoice
	for rows.Next() {
		var inv Invoice
		if err := rows.Scan(
			&inv.ID, &inv.UserID, &inv.Amount, &inv.BonusAmount, &inv.InvoiceType,
			&inv.InstanceID, &inv.PromoID, &inv.Status, &inv.Provider,
			&inv.ProviderPaymentID, &inv.PaymentURL, &inv.Description, &inv.BalanceAfter, &inv.CreatedAt,
			&inv.Region, &inv.PlanTier, &inv.PlanName, &inv.ProductType,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, inv)
	}
	return items, total, rows.Err()
}

func (s *Store) MarkInvoicePaidByID(ctx context.Context, invoiceID string, webhookAmount float64) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

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

func (s *Store) MarkInvoicePaid(ctx context.Context, providerPaymentID string, webhookAmount float64) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var invoiceID string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM billing.invoices WHERE provider_payment_id = $1 FOR UPDATE
	`, providerPaymentID).Scan(&invoiceID)
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

func (s *Store) MarkInvoiceFailedByID(ctx context.Context, invoiceID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE billing.invoices
		SET status = 'failed', updated_at = now()
		WHERE id = $1 AND status = 'pending'
	`, invoiceID)
	return err
}

func (s *Store) MarkInvoiceFailed(ctx context.Context, providerPaymentID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE billing.invoices
		SET status = 'failed', updated_at = now()
		WHERE provider_payment_id = $1 AND status = 'pending'
	`, providerPaymentID)
	return err
}
