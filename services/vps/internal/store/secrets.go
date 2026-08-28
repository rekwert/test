package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Store) sealSecret(plain string) (string, error) {
	if s.secrets == nil || plain == "" {
		return plain, nil
	}
	return s.secrets.Encrypt(plain)
}

func (s *Store) openSecret(stored string) (string, error) {
	if s.secrets == nil || stored == "" {
		return stored, nil
	}
	return s.secrets.Decrypt(stored)
}

func (s *Store) GetInstanceRootPassword(ctx context.Context, instanceID string) (string, error) {
	var stored string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(root_password, '') FROM vps.instances WHERE id = $1::uuid
	`, instanceID).Scan(&stored)
	if err != nil {
		return "", err
	}
	return s.openSecret(stored)
}

func (s *Store) SetPendingPasswordChange(ctx context.Context, instanceID, password string) error {
	sealed, err := s.sealSecret(strings.TrimSpace(password))
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE vps.instances
		SET pending_password_change = $2, updated_at = now()
		WHERE id = $1::uuid
	`, instanceID, strOrNil(sealed))
	return err
}

func (s *Store) TakePendingPasswordChange(ctx context.Context, instanceID string) (string, error) {
	var stored string
	err := s.pool.QueryRow(ctx, `
		UPDATE vps.instances
		SET pending_password_change = NULL, updated_at = now()
		WHERE id = $1::uuid AND pending_password_change IS NOT NULL
		RETURNING pending_password_change
	`, instanceID).Scan(&stored)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return s.openSecret(stored)
}

// ReencryptLegacySecrets seals plaintext root passwords and SSH private keys when
// VPS_FIELD_ENCRYPTION_KEY is configured.
func (s *Store) ReencryptLegacySecrets(ctx context.Context) error {
	if s.secrets == nil || !s.secrets.Enabled() {
		return nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id::text, COALESCE(root_password, '')
		FROM vps.instances
		WHERE root_password IS NOT NULL AND root_password <> ''
		  AND root_password NOT LIKE 'enc:v1:%'
		LIMIT 500
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, plain string
		if err := rows.Scan(&id, &plain); err != nil {
			return err
		}
		if err := s.UpdateInstanceRootPassword(ctx, id, plain); err != nil {
			return fmt.Errorf("reencrypt instance %s password: %w", id, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	keyRows, err := s.pool.Query(ctx, `
		SELECT host(ip)::text,
		       ed25519_private,
		       COALESCE(ecdsa_private, '')
		FROM vps.ip_ssh_host_keys
		WHERE ed25519_private IS NOT NULL AND ed25519_private <> ''
		  AND ed25519_private NOT LIKE 'enc:v1:%'
		LIMIT 500
	`)
	if err != nil {
		return err
	}
	defer keyRows.Close()
	for keyRows.Next() {
		var ip, edPriv, ecPriv string
		if err := keyRows.Scan(&ip, &edPriv, &ecPriv); err != nil {
			return err
		}
		stored, err := s.GetIPSSHHostKeys(ctx, ip)
		if err != nil || stored == nil {
			continue
		}
		if err := s.UpsertIPSSHHostKeys(ctx, *stored); err != nil {
			return fmt.Errorf("reencrypt ssh keys for %s: %w", ip, err)
		}
	}
	return keyRows.Err()
}
