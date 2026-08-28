package password

import (
	"fmt"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
	"golang.org/x/crypto/bcrypt"
)

const MinLength = 10

// BcryptCost returns 12 in production, default (10) otherwise.
func BcryptCost() int {
	if prodenv.IsProduction() {
		return 12
	}
	return bcrypt.DefaultCost
}

// Validate enforces minimum length.
func Validate(raw string) error {
	if len(raw) < MinLength {
		return fmt.Errorf("password must be at least %d characters", MinLength)
	}
	return nil
}

// Hash validates and returns a bcrypt hash.
func Hash(raw string) (string, error) {
	if err := Validate(raw); err != nil {
		return "", err
	}
	h, err := bcrypt.GenerateFromPassword([]byte(raw), BcryptCost())
	if err != nil {
		return "", err
	}
	return string(h), nil
}
