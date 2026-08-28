package store

import (
	"errors"
	"strings"
)

var (
	ErrInvalidRoleFilter          = errors.New("invalid role filter")
	ErrInvalidStatusFilter        = errors.New("invalid status filter")
	ErrInvalidEmailVerifiedFilter = errors.New("invalid email_verified filter")
)

func escapeLikePattern(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '\\', '%', '_':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func validUserRoleFilter(role string) bool {
	switch role {
	case "", "client", "support", "admin", "owner":
		return true
	default:
		return false
	}
}

func validBillingStatusFilter(status string) bool {
	switch status {
	case "", "active", "suspended":
		return true
	default:
		return false
	}
}

func validEmailVerifiedFilter(v string) bool {
	switch v {
	case "", "true", "false":
		return true
	default:
		return false
	}
}
