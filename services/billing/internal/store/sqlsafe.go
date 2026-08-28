package store

import (
	"errors"
	"strings"
)

var ErrInvalidBillingStatus = errors.New("invalid billing status")

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

func validBillingStatus(status string) bool {
	switch status {
	case "active", "past_due", "grace_period", "suspended", "cancelled":
		return true
	default:
		return false
	}
}
