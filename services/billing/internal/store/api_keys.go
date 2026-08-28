package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/apikey"
	"github.com/jackc/pgx/v5"
)

var ErrAPIKeyNotFound = errors.New("api key not found")

type APIKeyIdentity struct {
	KeyID  string
	UserID string
	Email  string
	Scopes []string
}

func HashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
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
			return nil, ErrAPIKeyNotFound
		}
		return nil, err
	}
	_, _ = s.pool.Exec(ctx, `UPDATE auth.api_keys SET last_used_at = now() WHERE id = $1::uuid`, id.KeyID)
	return &id, nil
}

func HasAPIKeyScope(scopes []string, need string) bool {
	if apikey.HasScope(scopes, need) {
		return true
	}
	switch need {
	case "billing", "billing.read", "billing.topup":
		return hasBillingScope(scopes)
	default:
		return false
	}
}

func hasBillingScope(scopes []string) bool {
	if apikey.HasScope(scopes, "billing") {
		return true
	}
	return apikey.HasScope(scopes, "billing.read") || apikey.HasScope(scopes, "billing.topup")
}
