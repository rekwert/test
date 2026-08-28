package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	APIKeyPrefix     = "ch_live_"
	MaxAPIKeysPerUser = 5
)

var AllowedAPIKeyScopes = map[string]struct{}{
	"billing":       {},
	"billing.read":  {},
	"billing.topup": {},
	"vps.read":      {},
	"vps.write":     {},
	"vps.manage":    {},
}

var ResellerAPIKeyScopes = []string{
	"billing.read",
	"billing.topup",
	"vps.read",
	"vps.write",
	"vps.manage",
}

type APIKey struct {
	ID         string
	UserID     string
	Name       string
	KeyPrefix  string
	Scopes     []string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

func NormalizeAPIKeyScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return []string{"billing"}, nil
	}
	seen := make(map[string]struct{}, len(scopes))
	out := make([]string, 0, len(scopes))
	for _, raw := range scopes {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if _, ok := AllowedAPIKeyScopes[s]; !ok {
			return nil, fmt.Errorf("unsupported scope: %s", s)
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one scope required")
	}
	return collapseCustomerBillingScopes(out), nil
}

func collapseCustomerBillingScopes(scopes []string) []string {
	hasVPS := false
	for _, s := range scopes {
		if strings.HasPrefix(s, "vps.") {
			hasVPS = true
			break
		}
	}
	if hasVPS {
		return scopes
	}
	hasBilling := false
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		switch s {
		case "billing", "billing.read", "billing.topup":
			hasBilling = true
		default:
			out = append(out, s)
		}
	}
	if hasBilling {
		out = append([]string{"billing"}, out...)
	}
	return out
}

func HashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func NewAPIKeySecret() (raw, prefix, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	raw = APIKeyPrefix + hex.EncodeToString(b)
	prefix = raw[:16]
	hash = HashAPIKey(raw)
	return raw, prefix, hash, nil
}

func (s *Store) CountActiveAPIKeys(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM auth.api_keys
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID).Scan(&n)
	return n, err
}

func (s *Store) ListAPIKeys(ctx context.Context, userID string) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, user_id::text, name, key_prefix, scopes, created_at, last_used_at
		FROM auth.api_keys
		WHERE user_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.Scopes, &k.CreatedAt, &k.LastUsedAt); err != nil {
			return nil, err
		}
		items = append(items, k)
	}
	return items, rows.Err()
}

func (s *Store) CreateAPIKey(ctx context.Context, userID, name, prefix, hash string, scopes []string) (*APIKey, error) {
	var k APIKey
	err := s.pool.QueryRow(ctx, `
		INSERT INTO auth.api_keys (user_id, name, key_prefix, key_hash, scopes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, user_id::text, name, key_prefix, scopes, created_at, last_used_at
	`, userID, name, prefix, hash, scopes).Scan(
		&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.Scopes, &k.CreatedAt, &k.LastUsedAt,
	)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (s *Store) RevokeAPIKey(ctx context.Context, userID, keyID string) error {
	res, err := s.pool.Exec(ctx, `
		UPDATE auth.api_keys
		SET revoked_at = now()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
	`, keyID, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

type APIKeyIdentity struct {
	KeyID  string
	UserID string
	Email  string
	Scopes []string
}

func (s *Store) ResolveAPIKey(ctx context.Context, rawKey string) (*APIKeyIdentity, error) {
	hash := HashAPIKey(rawKey)
	var id APIKeyIdentity
	err := s.pool.QueryRow(ctx, `
		SELECT k.id::text, k.user_id::text, u.email, k.scopes
		FROM auth.api_keys k
		JOIN auth.users u ON u.id = k.user_id
		WHERE k.key_hash = $1
		  AND k.revoked_at IS NULL
		  AND u.deleted_at IS NULL
	`, hash).Scan(&id.KeyID, &id.UserID, &id.Email, &id.Scopes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("api key not found")
		}
		return nil, err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE auth.api_keys SET last_used_at = now() WHERE id = $1::uuid`, id.KeyID)
	return &id, nil
}
