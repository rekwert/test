package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

const EmailChannel = "email"

type Delivery struct {
	ID       int64
	UserID   *string
	ToEmail  string
	Subject  string
	Body     string
	Template string
	Metadata json.RawMessage
	Attempts int
}

func (s *Store) EnqueueEmail(ctx context.Context, userID *string, to, subject, body, template string, metadata json.RawMessage) (int64, error) {
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	var userArg any
	if userID != nil && *userID != "" {
		userArg = *userID
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO notification.deliveries (user_id, channel, template, status, to_email, subject, body, metadata)
		VALUES ($1::uuid, 'email', $2, 'pending', $3, $4, $5, $6::jsonb)
		RETURNING id
	`, userArg, template, to, subject, body, metadata).Scan(&id)
	return id, err
}

func (s *Store) FetchPendingDeliveries(ctx context.Context, limit int) ([]Delivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		UPDATE notification.deliveries AS d
		SET status = 'processing', updated_at = now()
		WHERE d.id IN (
			SELECT id FROM notification.deliveries
			WHERE channel = 'email'
			  AND attempts < 5
			  AND (
			    status = 'pending'
			    OR (status = 'processing' AND updated_at < now() - interval '10 minutes')
			  )
			ORDER BY created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING d.id, d.user_id::text, COALESCE(d.to_email, ''), COALESCE(d.subject, ''), COALESCE(d.body, ''),
			COALESCE(d.template, ''), COALESCE(d.metadata, '{}'::jsonb), d.attempts
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Delivery
	for rows.Next() {
		var d Delivery
		var userID *string
		if err := rows.Scan(&d.ID, &userID, &d.ToEmail, &d.Subject, &d.Body, &d.Template, &d.Metadata, &d.Attempts); err != nil {
			return nil, err
		}
		d.UserID = userID
		items = append(items, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) MarkDeliverySent(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notification.deliveries
		SET status = 'sent', updated_at = now(), processed_at = now()
		WHERE id = $1
	`, id)
	return err
}

func (s *Store) MarkDeliveryFailed(ctx context.Context, id int64, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notification.deliveries
		SET attempts = attempts + 1,
		    last_error = $2,
		    status = CASE WHEN attempts + 1 >= 5 THEN 'failed' ELSE 'pending' END,
		    updated_at = now()
		WHERE id = $1
	`, id, errMsg)
	return err
}

func (s *Store) GetDeliveryByID(ctx context.Context, id int64) (*Delivery, error) {
	var d Delivery
	var userID *string
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id::text, COALESCE(to_email, ''), COALESCE(subject, ''), COALESCE(body, ''),
			COALESCE(template, ''), COALESCE(metadata, '{}'::jsonb), attempts
		FROM notification.deliveries WHERE id = $1
	`, id).Scan(&d.ID, &userID, &d.ToEmail, &d.Subject, &d.Body, &d.Template, &d.Metadata, &d.Attempts)
	if err == pgx.ErrNoRows {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	d.UserID = userID
	return &d, nil
}

func (s *Store) UserWantsEmail(ctx context.Context, userID string) (bool, error) {
	var notifyEmail bool
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(notify_email, true) FROM auth.users WHERE id = $1::uuid
	`, userID).Scan(&notifyEmail)
	if err == pgx.ErrNoRows {
		return true, nil
	}
	return notifyEmail, err
}

func DeliveryBackoff(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return 0
	case attempts == 2:
		return 30 * time.Second
	case attempts == 3:
		return 2 * time.Minute
	default:
		return 10 * time.Minute
	}
}
