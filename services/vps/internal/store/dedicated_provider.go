package store

import "strings"

func IsDedicatedProvider(provider string) bool {
	switch strings.TrimSpace(provider) {
	case "hetzner_robot", "hostkey":
		return true
	default:
		return false
	}
}
