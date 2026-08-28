package authn

import (
	"fmt"
	"strings"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/jwtauth"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID string   `json:"sub"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

func ClaimsFromRequest(authHeader, secret string) (*Claims, error) {
	if secret == "" {
		return nil, fmt.Errorf("auth not configured")
	}
	raw := strings.TrimSpace(authHeader)
	if raw == "" {
		return nil, fmt.Errorf("missing authorization")
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(raw, prefix) {
		return nil, fmt.Errorf("invalid authorization")
	}
	tokenStr := strings.TrimSpace(strings.TrimPrefix(raw, prefix))
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token")
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.UserID == "" {
		return nil, fmt.Errorf("invalid token")
	}
	if err := jwtauth.RequirePortalAudience(claims.Audience); err != nil {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

func IsStaff(roles []string) bool {
	for _, r := range roles {
		switch r {
		case "owner", "admin", "support":
			return true
		}
	}
	return false
}

func IsManager(roles []string) bool {
	for _, r := range roles {
		if r == "owner" || r == "admin" {
			return true
		}
	}
	return false
}
