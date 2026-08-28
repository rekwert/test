package jwtauth

import (
	"fmt"
	"strings"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"github.com/golang-jwt/jwt/v5"
)

const PortalAudience = "vps-portal"

// RequirePortalAudience rejects tokens not minted for the customer portal (e.g. telegram-bot).
func RequirePortalAudience(aud jwt.ClaimStrings) error {
	if !prodenv.IsProduction() {
		return nil
	}
	for _, a := range aud {
		switch strings.TrimSpace(a) {
		case PortalAudience:
			return nil
		case "telegram-bot":
			return fmt.Errorf("invalid token audience")
		}
	}
	return fmt.Errorf("invalid token audience")
}
