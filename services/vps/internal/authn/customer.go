package authn

import (
	"context"
	"fmt"
	"strings"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/apikey"
	"github.com/borishru-boop/testVPStrade/packages/shared-go/authuser"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CustomerIdentity struct {
	UserID    string
	Email     string
	Roles     []string
	Scopes    []string
	ViaAPIKey bool
}

type APIKeyResolver interface {
	ResolveAPIKey(ctx context.Context, rawKey string) (*store.APIKeyIdentity, error)
}

func AuthenticateCustomer(
	ctx context.Context,
	authHeader, apiKeyHeader, jwtSecret string,
	keys APIKeyResolver,
	pool *pgxpool.Pool,
	needScopes ...string,
) (*CustomerIdentity, error) {
	token := apikey.TokenFromRequest(authHeader, apiKeyHeader)
	if token == "" {
		return nil, fmt.Errorf("missing authorization")
	}

	if apikey.IsAPIKeyToken(token) {
		if keys == nil {
			return nil, fmt.Errorf("api keys unavailable")
		}
		ak, err := keys.ResolveAPIKey(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("invalid api key")
		}
		for _, need := range needScopes {
			if !apikey.HasScope(ak.Scopes, need) {
				return nil, fmt.Errorf("missing scope %s", need)
			}
		}
		if pool != nil {
			verified, err := authuser.EmailVerified(ctx, pool, ak.UserID)
			if err != nil {
				return nil, fmt.Errorf("lookup failed")
			}
			if !verified {
				return nil, fmt.Errorf("email not verified")
			}
		}
		return &CustomerIdentity{
			UserID:    ak.UserID,
			Email:     ak.Email,
			Scopes:    ak.Scopes,
			ViaAPIKey: true,
		}, nil
	}

	claims, err := ClaimsFromRequest(authHeader, jwtSecret)
	if err != nil {
		return nil, err
	}
	if IsStaff(claims.Roles) {
		return &CustomerIdentity{
			UserID: claims.UserID,
			Email:  claims.Email,
			Roles:  claims.Roles,
		}, nil
	}
	if pool != nil {
		verified, err := authuser.EmailVerified(ctx, pool, claims.UserID)
		if err != nil {
			return nil, fmt.Errorf("lookup failed")
		}
		if !verified {
			return nil, fmt.Errorf("email not verified")
		}
	}
	return &CustomerIdentity{
		UserID: claims.UserID,
		Email:  claims.Email,
		Roles:  claims.Roles,
	}, nil
}

func CustomerAuthErrorStatus(err error) (int, string) {
	if err == nil {
		return 0, ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "missing scope"):
		return 403, "insufficient scope"
	case strings.Contains(msg, "email not verified"):
		return 403, "email not verified"
	case strings.Contains(msg, "invalid api key"), strings.Contains(msg, "invalid token"), strings.Contains(msg, "missing authorization"), strings.Contains(msg, "invalid authorization"):
		return 401, "unauthorized"
	default:
		return 401, "unauthorized"
	}
}