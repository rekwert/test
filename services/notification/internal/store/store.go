package store

import (
	"context"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/dbpool"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/platformmigrate"
	"github.com/borishru-boop/testVPStrade/services/notification/internal/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InboxItem struct {
	ID        string
	UserID    string
	Title     string
	Body      string
	Category  string
	ReadAt    *time.Time
	CreatedAt time.Time
}

type Store struct {
	pool *pgxpool.Pool
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
	return platformmigrate.Apply(ctx, s.pool, "notification", migrations.FS)
}

func (s *Store) ListInbox(ctx context.Context, userID string, limit int) ([]InboxItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, user_id::text, title, body, category, read_at, created_at
		FROM notification.inbox
		WHERE user_id = $1::uuid
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []InboxItem
	for rows.Next() {
		var item InboxItem
		if err := rows.Scan(&item.ID, &item.UserID, &item.Title, &item.Body, &item.Category, &item.ReadAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) MarkRead(ctx context.Context, userID, notificationID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE notification.inbox
		SET read_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND read_at IS NULL
	`, notificationID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE notification.inbox
		SET read_at = now()
		WHERE user_id = $1::uuid AND read_at IS NULL
	`, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) CreateInboxMessages(ctx context.Context, userIDs []string, title, body, category, staffID string) (int, error) {
	if len(userIDs) == 0 {
		return 0, nil
	}
	if category == "" {
		category = "admin"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	count := 0
	for _, uid := range userIDs {
		if _, err := uuid.Parse(uid); err != nil {
			continue
		}
		var staffArg any
		if staffID != "" {
			staffArg = staffID
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO notification.inbox (user_id, title, body, category, sent_by_staff_id)
			VALUES ($1::uuid, $2, $3, $4, $5::uuid)
		`, uid, title, body, category, staffArg); err != nil {
			return 0, err
		}
		count++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) ListUserIDsByNode(ctx context.Context, nodeID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT i.user_id::text
		FROM vps.instances i
		WHERE i.node_id = $1::uuid
		  AND i.state <> 'deleted'
		  AND i.billing_status = 'active'
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) UserExists(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM auth.users WHERE id = $1::uuid)
	`, userID).Scan(&exists)
	return exists, err
}

func (s *Store) UserIsStaff(ctx context.Context, userID string) (bool, error) {
	var isStaff bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM auth.user_roles ur
			JOIN auth.roles r ON r.id = ur.role_id
			WHERE ur.user_id = $1::uuid
			  AND r.name IN ('owner', 'admin', 'support')
		)
	`, userID).Scan(&isStaff)
	return isStaff, err
}

func (s *Store) UserIDByEmail(ctx context.Context, email string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text FROM auth.users WHERE lower(email) = lower($1)
	`, strings.TrimSpace(email)).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) UserEmailByID(ctx context.Context, userID string) (string, error) {
	var email string
	err := s.pool.QueryRow(ctx, `
		SELECT email FROM auth.users WHERE id = $1::uuid
	`, userID).Scan(&email)
	if err != nil {
		return "", err
	}
	return email, nil
}

// TelegramChatIDForNotify returns the linked telegram chat when the user
// has connected @CloudHustle_Bot (telegram_id set).
func (s *Store) TelegramChatIDForNotify(ctx context.Context, userID string) (chatID int64, ok bool, err error) {
	var tgID *int64
	err = s.pool.QueryRow(ctx, `
		SELECT telegram_id
		FROM auth.users
		WHERE id = $1::uuid AND deleted_at IS NULL
	`, userID).Scan(&tgID)
	if err != nil {
		return 0, false, err
	}
	if tgID == nil || *tgID == 0 {
		return 0, false, nil
	}
	return *tgID, true, nil
}

func (s *Store) NodeExists(ctx context.Context, nodeID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM vps.nodes WHERE id = $1::uuid)
	`, nodeID).Scan(&exists)
	return exists, err
}
