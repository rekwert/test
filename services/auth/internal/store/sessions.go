package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type SessionMeta struct {
	IP         string
	ClientPort *int
	ServerPort *int
	UserAgent  string
	Browser    string
	OS         string
	DeviceType string
	AuthMethod string
}

type UserSession struct {
	ID           string
	Browser      string
	OS           string
	DeviceType   string
	AuthMethod   string
	IP           string
	LoggedInAt   time.Time
	LastActiveAt time.Time
}

func (s *Store) SaveRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time, meta SessionMeta) (string, error) {
	if meta.DeviceType == "" {
		meta.DeviceType = "desktop"
	}
	if meta.AuthMethod == "" {
		meta.AuthMethod = "password"
	}
	if meta.Browser == "" {
		meta.Browser = "Unknown"
	}
	if meta.OS == "" {
		meta.OS = "Unknown"
	}

	var ip any
	if meta.IP != "" {
		ip = meta.IP
	}

	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO auth.refresh_tokens (
			user_id, token_hash, expires_at, ip_address, client_port, server_port, user_agent,
			browser, os, device_type, auth_method, last_active_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
		RETURNING id::text
	`, userID, tokenHash, expiresAt, ip, meta.ClientPort, meta.ServerPort, meta.UserAgent, meta.Browser, meta.OS, meta.DeviceType, meta.AuthMethod).Scan(&id)
	return id, err
}

func (s *Store) ConsumeRefreshToken(ctx context.Context, tokenHash string) (userID string, meta SessionMeta, ok bool, err error) {
	err = s.pool.QueryRow(ctx, `
		UPDATE auth.refresh_tokens
		SET revoked_at = now(), last_active_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
		RETURNING user_id,
		          COALESCE(host(ip_address)::text, ''),
		          COALESCE(user_agent, ''),
		          browser, os, device_type, auth_method
	`, tokenHash).Scan(
		&userID, &meta.IP, &meta.UserAgent, &meta.Browser, &meta.OS, &meta.DeviceType, &meta.AuthMethod,
	)
	if err == pgx.ErrNoRows {
		return "", SessionMeta{}, false, nil
	}
	if err != nil {
		return "", SessionMeta{}, false, err
	}
	return userID, meta, true, nil
}

func (s *Store) TouchRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE auth.refresh_tokens
		SET last_active_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
	`, tokenHash)
	return err
}

func (s *Store) GetRefreshTokenMeta(ctx context.Context, tokenHash string) (SessionMeta, error) {
	var meta SessionMeta
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(host(ip_address)::text, ''), COALESCE(user_agent, ''),
		       browser, os, device_type, auth_method
		FROM auth.refresh_tokens
		WHERE token_hash = $1
	`, tokenHash).Scan(&meta.IP, &meta.UserAgent, &meta.Browser, &meta.OS, &meta.DeviceType, &meta.AuthMethod)
	return meta, err
}

func (s *Store) ListUserSessions(ctx context.Context, userID string) ([]UserSession, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, browser, os, device_type, auth_method,
		       COALESCE(host(ip_address)::text, ''), created_at, last_active_at
		FROM auth.refresh_tokens
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
		ORDER BY last_active_at DESC
		LIMIT 50
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []UserSession
	for rows.Next() {
		var sess UserSession
		if err := rows.Scan(
			&sess.ID, &sess.Browser, &sess.OS, &sess.DeviceType, &sess.AuthMethod,
			&sess.IP, &sess.LoggedInAt, &sess.LastActiveAt,
		); err != nil {
			return nil, err
		}
		items = append(items, sess)
	}
	return items, rows.Err()
}

func (s *Store) RevokeSession(ctx context.Context, userID, sessionID string) error {
	res, err := s.pool.Exec(ctx, `
		UPDATE auth.refresh_tokens
		SET revoked_at = now()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
	`, sessionID, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) RevokeOtherSessions(ctx context.Context, userID, currentSessionID string) (int64, error) {
	res, err := s.pool.Exec(ctx, `
		UPDATE auth.refresh_tokens
		SET revoked_at = now()
		WHERE user_id = $1 AND id <> $2 AND revoked_at IS NULL
	`, userID, currentSessionID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

func (s *Store) RevokeAllUserRefreshTokens(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE auth.refresh_tokens
		SET revoked_at = now()
		WHERE user_id = $1::uuid AND revoked_at IS NULL
	`, userID)
	return err
}
