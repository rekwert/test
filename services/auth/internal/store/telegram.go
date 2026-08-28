package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrTelegramTaken = errors.New("telegram already linked to another account")
	ErrLinkCode      = errors.New("invalid or expired link code")
)

func (s *Store) GetUserIDByTelegram(ctx context.Context, telegramID int64) (userID, email string, roles []string, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT id::text, email
		FROM auth.users
		WHERE telegram_id = $1 AND deleted_at IS NULL
	`, telegramID).Scan(&userID, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil, pgx.ErrNoRows
	}
	if err != nil {
		return "", "", nil, err
	}
	roles, err = s.getUserRoles(ctx, userID)
	return userID, email, roles, err
}

func (s *Store) GetTelegramID(ctx context.Context, userID string) (*int64, error) {
	var tg *int64
	err := s.pool.QueryRow(ctx, `
		SELECT telegram_id FROM auth.users WHERE id = $1::uuid AND deleted_at IS NULL
	`, userID).Scan(&tg)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return tg, err
}

func (s *Store) CreateTelegramLinkCode(ctx context.Context, userID string, telegramID int64, codeHash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM auth.telegram_link_codes WHERE telegram_id = $1 OR user_id = $2::uuid`, telegramID, userID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO auth.telegram_link_codes (user_id, telegram_id, code_hash, expires_at)
		VALUES ($1::uuid, $2, $3, $4)
	`, userID, telegramID, codeHash, expiresAt)
	return err
}

func (s *Store) ConfirmTelegramLink(ctx context.Context, telegramID int64, codeHash string) (userID, email string, roles []string, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", nil, err
	}
	defer tx.Rollback(ctx)

	var uid string
	err = tx.QueryRow(ctx, `
		SELECT user_id::text
		FROM auth.telegram_link_codes
		WHERE telegram_id = $1 AND code_hash = $2 AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 1
	`, telegramID, codeHash).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil, ErrLinkCode
	}
	if err != nil {
		return "", "", nil, err
	}

	var taken bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM auth.users
			WHERE telegram_id = $1 AND id <> $2::uuid AND deleted_at IS NULL
		)
	`, telegramID, uid).Scan(&taken)
	if err != nil {
		return "", "", nil, err
	}
	if taken {
		return "", "", nil, ErrTelegramTaken
	}

	_, err = tx.Exec(ctx, `
		UPDATE auth.users
		SET telegram_id = $1, notify_telegram = true, updated_at = now()
		WHERE id = $2::uuid
	`, telegramID, uid)
	if err != nil {
		return "", "", nil, err
	}
	_, _ = tx.Exec(ctx, `DELETE FROM auth.telegram_link_codes WHERE user_id = $1::uuid OR telegram_id = $2`, uid, telegramID)

	err = tx.QueryRow(ctx, `
		SELECT id::text, email FROM auth.users WHERE id = $1::uuid
	`, uid).Scan(&userID, &email)
	if err != nil {
		return "", "", nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", nil, err
	}
	roles, err = s.getUserRoles(ctx, userID)
	if err != nil {
		return "", "", nil, err
	}
	return userID, email, roles, nil
}

func (s *Store) UnlinkTelegram(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE auth.users SET telegram_id = NULL, notify_telegram = false, updated_at = now()
		WHERE id = $1::uuid
	`, userID)
	return err
}

func (s *Store) CreateTelegramWebLinkToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM auth.telegram_web_link_tokens WHERE user_id = $1::uuid`, userID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO auth.telegram_web_link_tokens (user_id, token_hash, expires_at)
		VALUES ($1::uuid, $2, $3)
	`, userID, tokenHash, expiresAt)
	return err
}

// ConfirmTelegramWebLink binds telegram_id using a one-time website token.
func (s *Store) ConfirmTelegramWebLink(ctx context.Context, telegramID int64, tokenHash string) (userID, email string, roles []string, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", nil, err
	}
	defer tx.Rollback(ctx)

	var uid string
	err = tx.QueryRow(ctx, `
		SELECT user_id::text
		FROM auth.telegram_web_link_tokens
		WHERE token_hash = $1 AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 1
	`, tokenHash).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil, ErrLinkCode
	}
	if err != nil {
		return "", "", nil, err
	}

	var taken bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM auth.users
			WHERE telegram_id = $1 AND id <> $2::uuid AND deleted_at IS NULL
		)
	`, telegramID, uid).Scan(&taken)
	if err != nil {
		return "", "", nil, err
	}
	if taken {
		return "", "", nil, ErrTelegramTaken
	}

	_, err = tx.Exec(ctx, `
		UPDATE auth.users
		SET telegram_id = $1, notify_telegram = true, updated_at = now()
		WHERE id = $2::uuid
	`, telegramID, uid)
	if err != nil {
		return "", "", nil, err
	}
	_, _ = tx.Exec(ctx, `DELETE FROM auth.telegram_web_link_tokens WHERE user_id = $1::uuid`, uid)
	_, _ = tx.Exec(ctx, `DELETE FROM auth.telegram_link_codes WHERE user_id = $1::uuid OR telegram_id = $2`, uid, telegramID)

	err = tx.QueryRow(ctx, `
		SELECT id::text, email FROM auth.users WHERE id = $1::uuid
	`, uid).Scan(&userID, &email)
	if err != nil {
		return "", "", nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", nil, err
	}
	roles, err = s.getUserRoles(ctx, userID)
	if err != nil {
		return "", "", nil, err
	}
	return userID, email, roles, nil
}
