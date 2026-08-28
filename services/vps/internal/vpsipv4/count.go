package vpsipv4

import (
	"encoding/json"
)

// ExtraCountOnInstance returns how many non-primary IPv4 are bound (all_ips minus primary).
func ExtraCountOnInstance(providerMeta json.RawMessage) int {
	if len(providerMeta) < 3 {
		return 0
	}
	var meta map[string]any
	if err := json.Unmarshal(providerMeta, &meta); err != nil {
		return 0
	}
	raw, ok := meta["all_ips"].([]any)
	if !ok || len(raw) <= 1 {
		return 0
	}
	return len(raw) - 1
}
