package store

import (
	"math"
	"time"
)

type UpgradeQuote struct {
	Amount            float64
	DeltaMonthly      float64
	RemainingDays     float64
	BillingPeriodDays int
	NextBillingAt     *time.Time
	FromPlan          string
	ToPlan            string
	FromPlanID        string
	ToPlanID          string
}

func calcUpgradeRemainingDays(now time.Time, nextBilling *time.Time, createdAt time.Time, periodDays int) float64 {
	if periodDays <= 0 {
		periodDays = 30
	}
	maxDays := float64(periodDays)

	var periodEnd time.Time
	switch {
	case nextBilling != nil:
		periodEnd = *nextBilling
	case !createdAt.IsZero():
		periodEnd = createdAt.Add(time.Duration(periodDays) * 24 * time.Hour)
	default:
		return maxDays
	}

	hours := periodEnd.Sub(now).Hours()
	if hours <= 0 {
		return 1
	}
	days := hours / 24
	if days < 1 {
		days = 1
	}
	if days > maxDays {
		days = maxDays
	}
	return days
}

func calcProratedUpgradeAmount(delta, remainingDays float64, periodDays int) float64 {
	if delta <= 0 {
		return 0
	}
	if periodDays <= 0 {
		periodDays = 30
	}
	amount := math.Round(delta*remainingDays/float64(periodDays)*100) / 100
	if delta > 0 && amount < 0.01 {
		amount = 0.01
	}
	return amount
}
