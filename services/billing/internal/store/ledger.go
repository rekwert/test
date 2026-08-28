package store

import (
	"context"
	"time"
)

type LedgerEntry struct {
	ID          string
	Kind        string
	Amount      float64
	Direction   string
	Status      string
	Provider    string
	Description string
	InstanceID  string
	CreatedAt   time.Time
}

func (s *Store) ListLedger(ctx context.Context, userID string, limit int) ([]LedgerEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, kind, amount, direction, status, provider, description, instance_id, created_at
		FROM (
			SELECT
				i.id::text,
				CASE
					WHEN i.invoice_type = 'charge' THEN 'charge'
					ELSE 'topup'
				END AS kind,
				i.amount::float8,
				CASE
					WHEN i.invoice_type = 'charge' THEN 'debit'
					ELSE 'credit'
				END AS direction,
				i.status,
				COALESCE(i.provider, ''),
				COALESCE(i.description, ''),
				COALESCE(i.instance_id::text, ''),
				i.created_at
			FROM billing.invoices i
			WHERE i.user_id = $1
			UNION ALL
			SELECT
				a.id::text,
				a.kind,
				a.amount::float8,
				CASE
					WHEN a.kind IN ('credit', 'refund') THEN 'credit'
					ELSE 'debit'
				END AS direction,
				'posted',
				'manual',
				COALESCE(a.reason, ''),
				'',
				a.created_at
			FROM billing.adjustments a
			WHERE a.user_id = $1
		) t
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []LedgerEntry
	for rows.Next() {
		var row LedgerEntry
		if err := rows.Scan(
			&row.ID, &row.Kind, &row.Amount, &row.Direction, &row.Status,
			&row.Provider, &row.Description, &row.InstanceID, &row.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	return items, rows.Err()
}
