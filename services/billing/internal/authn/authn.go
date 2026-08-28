package authn

import (
	"context"
	"fmt"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/apikey"
	"github.com/borishru-boop/testVPStrade/services/billing/internal/store"
)

const APIKeyBearerPrefix = "ch_live_"

type Identity struct {
	UserID    string
	Email     string
	Roles     []string
	Scopes    []string
	ViaAPIKey bool
}

type apiKeyResolver interface {
	ResolveAPIKey(ctx context.Context, rawKey string) (*store.APIKeyIdentity, error)
}

// Authenticate accepts session JWT or customer API key (ch_live_…).
// needScopes are required only for API keys; JWT session has full customer access.
func Authenticate(ctx context.Context, authHeader, apiKeyHeader, secret string, keys apiKeyResolver, needScopes ...string) (*Identity, error) {
	tokenStr := apikey.TokenFromRequest(authHeader, apiKeyHeader)
	if tokenStr == "" {
		return nil, fmt.Errorf("missing authorization")
	}

	if apikey.IsAPIKeyToken(tokenStr) {
		if keys == nil {
			return nil, fmt.Errorf("api keys unavailable")
		}
		ak, err := keys.ResolveAPIKey(ctx, tokenStr)
		if err != nil {
			return nil, fmt.Errorf("invalid api key")
		}
		for _, need := range needScopes {
			if !store.HasAPIKeyScope(ak.Scopes, need) {
				return nil, fmt.Errorf("missing scope %s", need)
			}
		}
		return &Identity{
			UserID:    ak.UserID,
			Email:     ak.Email,
			Scopes:    ak.Scopes,
			ViaAPIKey: true,
		}, nil
	}

	claims, err := ClaimsFromRequest(authHeader, secret)
	if err != nil {
		return nil, err
	}
	return &Identity{
		UserID: claims.UserID,
		Email:  claims.Email,
		Roles:  claims.Roles,
	}, nil
}
