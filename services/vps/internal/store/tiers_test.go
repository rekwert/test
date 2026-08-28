package store

import "testing"

func TestTierAcceptsCapacityWaitlist(t *testing.T) {
	tests := []struct {
		tier string
		want bool
	}{
		{"midrange", true},
		{"MIDRANGE", true},
		{"hustle", true},
		{"prosto", false},
		{"custom", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := TierAcceptsCapacityWaitlist(tc.tier); got != tc.want {
			t.Errorf("TierAcceptsCapacityWaitlist(%q) = %v, want %v", tc.tier, got, tc.want)
		}
	}
}
