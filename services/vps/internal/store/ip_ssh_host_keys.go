package store

import (
	"context"
	"errors"
	"net"

	"github.com/jackc/pgx/v5"
)

type IPSShHostKeys struct {
	IP             string
	Ed25519Private string
	Ed25519Public  string
	ECDSAPrivate   string
	ECDSAPublic    string
}

func (s *Store) GetIPSSHHostKeys(ctx context.Context, ip string) (*IPSShHostKeys, error) {
	ip = normalizeIP(ip)
	if ip == "" {
		return nil, nil
	}
	var row IPSShHostKeys
	err := s.pool.QueryRow(ctx, `
		SELECT host(ip), ed25519_private, ed25519_public,
		       COALESCE(ecdsa_private, ''), COALESCE(ecdsa_public, '')
		FROM vps.ip_ssh_host_keys
		WHERE ip = $1::inet
	`, ip).Scan(&row.IP, &row.Ed25519Private, &row.Ed25519Public, &row.ECDSAPrivate, &row.ECDSAPublic)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	row.Ed25519Private, err = s.openSecret(row.Ed25519Private)
	if err != nil {
		return nil, err
	}
	if row.ECDSAPrivate != "" {
		row.ECDSAPrivate, err = s.openSecret(row.ECDSAPrivate)
		if err != nil {
			return nil, err
		}
	}
	return &row, nil
}

func (s *Store) UpsertIPSSHHostKeys(ctx context.Context, keys IPSShHostKeys) error {
	keys.IP = normalizeIP(keys.IP)
	if keys.IP == "" || keys.Ed25519Private == "" || keys.Ed25519Public == "" {
		return nil
	}
	edPriv, err := s.sealSecret(keys.Ed25519Private)
	if err != nil {
		return err
	}
	ecPriv := keys.ECDSAPrivate
	if ecPriv != "" {
		ecPriv, err = s.sealSecret(ecPriv)
		if err != nil {
			return err
		}
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO vps.ip_ssh_host_keys (ip, ed25519_private, ed25519_public, ecdsa_private, ecdsa_public, updated_at)
		VALUES ($1::inet, $2, $3, NULLIF($4, ''), NULLIF($5, ''), now())
		ON CONFLICT (ip) DO UPDATE SET
			ed25519_private = EXCLUDED.ed25519_private,
			ed25519_public = EXCLUDED.ed25519_public,
			ecdsa_private = COALESCE(EXCLUDED.ecdsa_private, vps.ip_ssh_host_keys.ecdsa_private),
			ecdsa_public = COALESCE(EXCLUDED.ecdsa_public, vps.ip_ssh_host_keys.ecdsa_public),
			updated_at = now()
	`, keys.IP, edPriv, keys.Ed25519Public, ecPriv, keys.ECDSAPublic)
	return err
}

func normalizeIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	return parsed.String()
}
