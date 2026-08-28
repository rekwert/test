package store

import (
	"context"
	"fmt"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/dbpool"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/platformmigrate"
	"github.com/borishru-boop/testVPStrade/services/auth/internal/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID              string
	Email           string
	PasswordHash    string
	EmailVerified   bool
	Locale          string
	DisplayName     string
	NotifyEmail     bool
	NotifyTelegram  bool
	TelegramLinked  bool
	Roles           []string
	CreatedAt       time.Time
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
	return platformmigrate.Apply(ctx, s.pool, "auth", migrations.FS)
}

func (s *Store) CreateUser(ctx context.Context, email, passwordHash, locale string) (*User, error) {
	if locale == "" {
		locale = "en"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var user User
	err = tx.QueryRow(ctx,
		`INSERT INTO auth.users (email, password_hash, locale) VALUES ($1, $2, $3)
		 RETURNING id, email, password_hash, email_verified, locale, COALESCE(display_name, ''),
		           COALESCE(notify_email, true), COALESCE(notify_telegram, false),
		           (telegram_id IS NOT NULL), created_at`,
		email, passwordHash, locale,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.EmailVerified, &user.Locale, &user.DisplayName,
		&user.NotifyEmail, &user.NotifyTelegram, &user.TelegramLinked, &user.CreatedAt)
	if err != nil {
		return nil, err
	}

	var roleID int
	err = tx.QueryRow(ctx, `SELECT id FROM auth.roles WHERE name = 'client'`).Scan(&roleID)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO auth.user_roles (user_id, role_id) VALUES ($1, $2)`, user.ID, roleID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	user.Roles = []string{"client"}
	return &user, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, email_verified, locale, COALESCE(display_name, ''),
		        COALESCE(notify_email, true), COALESCE(notify_telegram, false),
		        (telegram_id IS NOT NULL), created_at
		 FROM auth.users WHERE email = $1 AND deleted_at IS NULL`, email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.EmailVerified, &user.Locale, &user.DisplayName,
		&user.NotifyEmail, &user.NotifyTelegram, &user.TelegramLinked, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	roles, err := s.getUserRoles(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	user.Roles = roles
	return &user, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*User, error) {
	var user User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, email_verified, locale, COALESCE(display_name, ''),
		        COALESCE(notify_email, true), COALESCE(notify_telegram, false),
		        (telegram_id IS NOT NULL), created_at
		 FROM auth.users WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.EmailVerified, &user.Locale, &user.DisplayName,
		&user.NotifyEmail, &user.NotifyTelegram, &user.TelegramLinked, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	roles, err := s.getUserRoles(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	user.Roles = roles
	return &user, nil
}

func (s *Store) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE auth.users SET password_hash = $2, updated_at = now() WHERE id = $1`, userID, passwordHash)
	return err
}

func (s *Store) MarkEmailVerified(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE auth.users SET email_verified = true, updated_at = now() WHERE id = $1`, userID)
	return err
}

func (s *Store) EmailVerificationIsStale(ctx context.Context, userID string, minAge time.Duration) (bool, error) {
	if minAge <= 0 {
		return true, nil
	}
	var stale bool
	err := s.pool.QueryRow(ctx, `
		SELECT NOT EXISTS (
			SELECT 1 FROM auth.email_verifications
			WHERE user_id = $1
			  AND used_at IS NULL
			  AND expires_at > now()
			  AND created_at > now() - ($2::text)::interval
		)
	`, userID, fmt.Sprintf("%d seconds", int(minAge.Seconds()))).Scan(&stale)
	return stale, err
}

func (s *Store) SaveEmailVerification(ctx context.Context, userID, codeHash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE auth.email_verifications SET used_at = now() WHERE user_id = $1 AND used_at IS NULL`, userID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO auth.email_verifications (user_id, code_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, codeHash, expiresAt)
	return err
}

func (s *Store) VerifyEmailCode(ctx context.Context, userID, codeHash string) error {
	var id string
	var attempts int
	var expiresAt time.Time
	var usedAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, attempts, expires_at, used_at FROM auth.email_verifications
		 WHERE user_id = $1 AND used_at IS NULL ORDER BY created_at DESC LIMIT 1`, userID,
	).Scan(&id, &attempts, &expiresAt, &usedAt)
	if err != nil {
		return err
	}
	if usedAt != nil || time.Now().After(expiresAt) {
		return pgx.ErrNoRows
	}
	if attempts >= 5 {
		return fmt.Errorf("too many attempts")
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE auth.email_verifications SET attempts = attempts + 1 WHERE id = $1`, id); err != nil {
		return err
	}
	var storedHash string
	err = s.pool.QueryRow(ctx, `SELECT code_hash FROM auth.email_verifications WHERE id = $1`, id).Scan(&storedHash)
	if err != nil {
		return err
	}
	if storedHash != codeHash {
		return pgx.ErrNoRows
	}
	_, err = s.pool.Exec(ctx, `UPDATE auth.email_verifications SET used_at = now() WHERE id = $1`, id)
	if err != nil {
		return err
	}
	return s.MarkEmailVerified(ctx, userID)
}

func (s *Store) SavePasswordReset(ctx context.Context, userID, codeHash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE auth.password_resets SET used_at = now() WHERE user_id = $1 AND used_at IS NULL`, userID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO auth.password_resets (user_id, code_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, codeHash, expiresAt)
	return err
}

func (s *Store) ResetPasswordWithCode(ctx context.Context, userID, codeHash, passwordHash string) error {
	var id string
	var attempts int
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, attempts, expires_at FROM auth.password_resets
		 WHERE user_id = $1 AND used_at IS NULL ORDER BY created_at DESC LIMIT 1`, userID,
	).Scan(&id, &attempts, &expiresAt)
	if err != nil {
		return err
	}
	if time.Now().After(expiresAt) || attempts >= 5 {
		return pgx.ErrNoRows
	}
	var storedHash string
	err = s.pool.QueryRow(ctx, `SELECT code_hash FROM auth.password_resets WHERE id = $1`, id).Scan(&storedHash)
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE auth.password_resets SET attempts = attempts + 1 WHERE id = $1`, id); err != nil {
		return err
	}
	if storedHash != codeHash {
		return pgx.ErrNoRows
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE auth.password_resets SET used_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE auth.users SET password_hash = $2, updated_at = now() WHERE id = $1`, userID, passwordHash); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) getUserRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT r.name FROM auth.roles r
		 JOIN auth.user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		roles = append(roles, name)
	}
	return roles, rows.Err()
}

func (s *Store) getUserRolesBatch(ctx context.Context, userIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT ur.user_id::text, r.name
		FROM auth.user_roles ur
		JOIN auth.roles r ON r.id = ur.role_id
		WHERE ur.user_id = ANY($1::uuid[])
		ORDER BY ur.user_id, r.name
	`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var userID, role string
		if err := rows.Scan(&userID, &role); err != nil {
			return nil, err
		}
		out[userID] = append(out[userID], role)
	}
	return out, rows.Err()
}

func (s *Store) GetRefreshToken(ctx context.Context, tokenHash string) (userID string, expiresAt time.Time, revoked bool, err error) {
	var revokedAt *time.Time
	err = s.pool.QueryRow(ctx,
		`SELECT user_id, expires_at, revoked_at FROM auth.refresh_tokens WHERE token_hash = $1`, tokenHash,
	).Scan(&userID, &expiresAt, &revokedAt)
	if err != nil {
		return "", time.Time{}, false, err
	}
	return userID, expiresAt, revokedAt != nil, nil
}

func (s *Store) RevokeRefreshToken(ctx context.Context, tokenHash string) (userID, sessionID string, err error) {
	err = s.pool.QueryRow(ctx, `
		UPDATE auth.refresh_tokens
		SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
		RETURNING user_id::text, id::text
	`, tokenHash).Scan(&userID, &sessionID)
	if err == pgx.ErrNoRows {
		return "", "", nil
	}
	return userID, sessionID, err
}

// CountUsersSince returns non-deleted email-verified users created at or after since (promo counter).
// If since is zero, counts all non-deleted verified users.
func (s *Store) CountUsersSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	var err error
	if since.IsZero() {
		err = s.pool.QueryRow(ctx,
			`SELECT COUNT(*)::int FROM auth.users WHERE deleted_at IS NULL AND email_verified = true`,
		).Scan(&n)
	} else {
		err = s.pool.QueryRow(ctx,
			`SELECT COUNT(*)::int FROM auth.users WHERE deleted_at IS NULL AND email_verified = true AND created_at >= $1`,
			since.UTC(),
		).Scan(&n)
	}
	return n, err
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}

func IsTooManyAttempts(err error) bool {
	return err != nil && err.Error() == "too many attempts"
}
