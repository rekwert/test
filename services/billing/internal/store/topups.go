package store

import (
	"context"
	"time"
)

type AdminTopupRow struct {
	ID          string
	UserID      string
	UserEmail   string
	Amount      float64
	Currency    string
	Status      string
	Provider    *string
	Description *string
	CreatedAt   time.Time
}

func (s *Store) ListPaidTopups(ctx context.Context, period StatsPeriod, limit, offset int) ([]AdminTopupRow, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	toExclusive := period.To.AddDate(0, 0, 1)

	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM billing.invoices
		WHERE status = 'paid'
		  AND invoice_type = 'topup'
		  AND created_at >= $1
		  AND created_at < $2
	`, period.From, toExclusive).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text,
			i.user_id::text,
			COALESCE(u.email, ''),
			i.amount::float8,
			i.status,
			i.provider,
			i.description,
			i.created_at
		FROM billing.invoices i
		LEFT JOIN auth.users u ON u.id = i.user_id
		WHERE i.status = 'paid'
		  AND i.invoice_type = 'topup'
		  AND i.created_at >= $1
		  AND i.created_at < $2
		ORDER BY i.created_at DESC
		LIMIT $3 OFFSET $4
	`, period.From, toExclusive, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]AdminTopupRow, 0, limit)
	for rows.Next() {
		var row AdminTopupRow
		if err := rows.Scan(
			&row.ID, &row.UserID, &row.UserEmail, &row.Amount, &row.Status,
			&row.Provider, &row.Description, &row.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		row.Currency = "RUB"
		out = append(out, row)
	}
	return out, total, rows.Err()
}
