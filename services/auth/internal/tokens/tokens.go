package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/jwtauth"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID         string   `json:"sub"`
	Email          string   `json:"email"`
	Roles          []string `json:"roles"`
	ImpersonatedBy string   `json:"imp,omitempty"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func New(secret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

func (m *Manager) AccessTTL() time.Duration  { return m.accessTTL }
func (m *Manager) RefreshTTL() time.Duration { return m.refreshTTL }

func (m *Manager) IssueAccessWithTTL(userID, email string, roles []string, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		return m.IssueAccess(userID, email, roles)
	}
	exp := time.Now().Add(ttl)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		Email:  email,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Audience:  jwt.ClaimStrings{"telegram-bot"},
		},
	})
	s, err := token.SignedString(m.secret)
	return s, exp, err
}

func (m *Manager) IssueAccess(userID, email string, roles []string) (string, time.Time, error) {
	exp := time.Now().Add(m.accessTTL)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		Email:  email,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "cloud-hustle-auth",
			Audience:  jwt.ClaimStrings{"vps-portal"},
		},
	})
	s, err := token.SignedString(m.secret)
	return s, exp, err
}

func (m *Manager) IssueAccessImpersonation(staffID, userID, email string, roles []string) (string, time.Time, error) {
	exp := time.Now().Add(m.accessTTL)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID:         userID,
		Email:          email,
		Roles:          roles,
		ImpersonatedBy: staffID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "cloud-hustle-auth",
			Audience:  jwt.ClaimStrings{"vps-portal"},
		},
	})
	s, err := token.SignedString(m.secret)
	return s, exp, err
}

func (m *Manager) ParseAccess(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	if err := jwtauth.RequirePortalAudience(claims.Audience); err != nil {
		return nil, err
	}
	return claims, nil
}

func (m *Manager) NewRefreshToken() (raw string, hash string, expiresAt time.Time, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", time.Time{}, err
	}
	raw = hex.EncodeToString(b)
	hash = HashRefresh(raw)
	expiresAt = time.Now().Add(m.refreshTTL)
	return raw, hash, expiresAt, nil
}

func HashRefresh(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
