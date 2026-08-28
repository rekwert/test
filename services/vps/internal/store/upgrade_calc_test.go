package store

import (
	"math"
	"testing"
	"time"
)

func TestCalcUpgradeRemainingDays(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	period := 30

	t.Run("future billing date", func(t *testing.T) {
		next := now.Add(5 * 24 * time.Hour)
		got := calcUpgradeRemainingDays(now, &next, time.Time{}, period)
		if math.Abs(got-5) > 0.01 {
			t.Fatalf("expected ~5 days, got %v", got)
		}
	})

	t.Run("overdue billing charges one day", func(t *testing.T) {
		next := now.Add(-2 * 24 * time.Hour)
		got := calcUpgradeRemainingDays(now, &next, time.Time{}, period)
		if got != 1 {
			t.Fatalf("expected 1 day, got %v", got)
		}
	})

	t.Run("null billing uses created_at", func(t *testing.T) {
		created := now.Add(-10 * 24 * time.Hour)
		got := calcUpgradeRemainingDays(now, nil, created, period)
		if math.Abs(got-20) > 0.01 {
			t.Fatalf("expected ~20 days, got %v", got)
		}
	})
}

func TestCalcProratedUpgradeAmount(t *testing.T) {
	got := calcProratedUpgradeAmount(300, 5, 30)
	want := 50.0
	if got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}

	tiny := calcProratedUpgradeAmount(1, 0.1, 30)
	if tiny != 0.01 {
		t.Fatalf("expected minimum 0.01, got %v", tiny)
	}
}
