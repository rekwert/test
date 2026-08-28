package store

import "strings"

// TierAcceptsCapacityWaitlist reports product lines that always queue on order
// (midrange, hustle). Promotion from the queue requires dedicated node hardware
// (see PromoteWaitlisted tier filter — no prosto on the same node).
func TierAcceptsCapacityWaitlist(tier string) bool {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "midrange", "hustle":
		return true
	default:
		return false
	}
}
