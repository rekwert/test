package store

import (
	"context"
	"strings"
	"time"
)

type BusinessStats struct {
	MRR               float64 `json:"mrr"`
	ARR               float64 `json:"arr"`
	ActiveSubscribers int     `json:"active_subscribers"`
	ChurnRate30d      float64 `json:"churn_rate_30d"`
	ChurnedUsers30d   int     `json:"churned_users_30d"`
	LTV               float64 `json:"ltv"`
	ARPU              float64 `json:"arpu"`
	Revenue30d        float64 `json:"revenue_30d"`
	RevenuePeriod     float64 `json:"revenue_period"`
	PeriodDays        int     `json:"period_days"`
	PeriodFrom        string  `json:"period_from"`
	PeriodTo          string  `json:"period_to"`
}

type StatsPeriod struct {
	From time.Time
	To   time.Time // inclusive calendar day (end exclusive = To+1day)
	Days int
}

func NewStatsPeriod(days int, fromRaw, toRaw string) StatsPeriod {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	parseDay := func(raw string) (time.Time, bool) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return time.Time{}, false
		}
		t, err := time.ParseInLocation("2006-01-02", raw, time.UTC)
		if err != nil {
			return time.Time{}, false
		}
		return t, true
	}

	from, okFrom := parseDay(fromRaw)
	to, okTo := parseDay(toRaw)
	if okFrom && okTo {
		if to.Before(from) {
			from, to = to, from
		}
		// Cap range at 366 days.
		if to.Sub(from) > 366*24*time.Hour {
			from = to.AddDate(0, 0, -365)
		}
		days := int(to.Sub(from).Hours()/24) + 1
		return StatsPeriod{From: from, To: to, Days: days}
	}

	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	from = today.AddDate(0, 0, -(days - 1))
	return StatsPeriod{From: from, To: today, Days: days}
}

func (s *Store) BusinessStats(ctx context.Context, period StatsPeriod) (*BusinessStats, error) {
	var stats BusinessStats
	stats.PeriodDays = period.Days
	stats.PeriodFrom = period.From.Format("2006-01-02")
	stats.PeriodTo = period.To.Format("2006-01-02")
	toExclusive := period.To.AddDate(0, 0, 1)

	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE((
				SELECT SUM(p.price_monthly)::float8
				FROM vps.instances i
				JOIN vps.plans p ON p.id = i.plan_id
				WHERE i.billing_status IN ('active', 'grace_period')
				  AND i.state NOT IN ('deleted', 'creating')
			), 0),
			COALESCE((
				SELECT COUNT(DISTINCT i.user_id)::int
				FROM vps.instances i
				WHERE i.billing_status IN ('active', 'grace_period')
				  AND i.state NOT IN ('deleted', 'creating')
			), 0),
			COALESCE((
				SELECT COUNT(DISTINCT user_id)::int
				FROM vps.instances
				WHERE billing_status = 'cancelled'
				  AND state = 'deleted'
				  AND updated_at >= now() - interval '30 days'
			), 0),
			COALESCE((
				SELECT SUM(amount)::float8
				FROM billing.invoices
				WHERE status = 'paid'
				  AND invoice_type = 'topup'
				  AND created_at >= now() - interval '30 days'
			), 0),
			COALESCE((
				SELECT SUM(amount)::float8
				FROM billing.invoices
				WHERE status = 'paid'
				  AND invoice_type = 'topup'
				  AND created_at >= $1
				  AND created_at < $2
			), 0)
	`, period.From, toExclusive).Scan(
		&stats.MRR, &stats.ActiveSubscribers, &stats.ChurnedUsers30d, &stats.Revenue30d, &stats.RevenuePeriod,
	)
	if err != nil {
		return nil, err
	}

	var totalUsers int
	_ = s.pool.QueryRow(ctx, `SELECT GREATEST(COUNT(*)::int, 1) FROM auth.users`).Scan(&totalUsers)
	stats.ChurnRate30d = float64(stats.ChurnedUsers30d) / float64(totalUsers) * 100
	stats.ARR = stats.MRR * 12
	if stats.ActiveSubscribers > 0 {
		stats.ARPU = stats.MRR / float64(stats.ActiveSubscribers)
		var avgLifetimeMonths float64
		_ = s.pool.QueryRow(ctx, `
			SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (
				COALESCE(CASE WHEN state = 'deleted' THEN updated_at END, now()) - created_at
			)) / 2592000), 6)::float8
			FROM vps.instances
		`).Scan(&avgLifetimeMonths)
		if avgLifetimeMonths < 1 {
			avgLifetimeMonths = 6
		}
		stats.LTV = stats.ARPU * avgLifetimeMonths
	}

	return &stats, nil
}
