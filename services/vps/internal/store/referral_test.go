package store

import (
	"math"
	"testing"
)

func TestReferralCommissionPercent(t *testing.T) {
	if referrerPercent != 10 {
		t.Fatalf("referrerPercent = %d, want 10", referrerPercent)
	}

	cases := []struct {
		amount float64
		want   float64
	}{
		{1000, 100},
		{365, 36.5},
		{500, 50},
		{0, 0},
		{1, 0.1},
	}
	for _, tc := range cases {
		got := math.Round(tc.amount*float64(referrerPercent)) / 100
		if got != tc.want {
			t.Fatalf("amount=%.2f: commission=%.2f, want %.2f", tc.amount, got, tc.want)
		}
	}
}
