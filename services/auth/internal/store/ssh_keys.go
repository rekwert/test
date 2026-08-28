package store

import (
	"context"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/sshpubkey"
	"github.com/jackc/pgx/v5"
)

type SSHKey struct {
	ID        string
	UserID    string
	Name      string
	PublicKey string
	CreatedAt time.Time
}

// ValidSSHPublicKey accepts only parseable OpenSSH authorized_keys lines.
func ValidSSHPublicKey(key string) bool {
	return sshpubkey.Valid(key)
}

func (s *Store) UpdateDisplayName(ctx context.Context, userID, name string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE auth.users SET display_name = $2, updated_at = now() WHERE id = $1`, userID, name)
	return err
}

func (s *Store) ListSSHKeys(ctx context.Context, userID string) ([]SSHKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, user_id::text, name, public_key, created_at
		FROM auth.ssh_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SSHKey
	for rows.Next() {
		var k SSHKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.PublicKey, &k.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, k)
	}
	return items, rows.Err()
}

func (s *Store) CreateSSHKey(ctx context.Context, userID, name, publicKey string) (*SSHKey, error) {
	var k SSHKey
	err := s.pool.QueryRow(ctx, `
		INSERT INTO auth.ssh_keys (user_id, name, public_key)
		VALUES ($1, $2, $3)
		RETURNING id::text, user_id::text, name, public_key, created_at
	`, userID, name, publicKey).Scan(&k.ID, &k.UserID, &k.Name, &k.PublicKey, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (s *Store) DeleteSSHKey(ctx context.Context, userID, keyID string) error {
	res, err := s.pool.Exec(ctx,
		`DELETE FROM auth.ssh_keys WHERE id = $1 AND user_id = $2`, keyID, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
