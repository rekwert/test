package hetznerrobot

import (
	"context"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	User          string
	Password      string
	BaseURL       string
	Enabled       bool
	MarkupPercent float64
	EurRub        float64
	SyncInterval  time.Duration
	PriceSlackPct float64
	// Extra single IPv4 (Robot server_addon additional_ipv4).
	// Prefer fixed RUB sell prices; EUR fields are cost/reference fallback only.
	ExtraIPv4EUR        float64
	ExtraIPv4SetupEUR   float64
	ExtraIPv4MonthlyRub float64
	ExtraIPv4SetupRub   float64
	ExtraIPv4Max        int
}

func LoadConfig() Config {
	markup, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HETZNER_MARKUP_PERCENT")), 64)
	if markup < 0 {
		markup = 0
	}
	fallback, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HETZNER_EUR_RUB")), 64)
	if fallback <= 0 {
		fallback = 100
	}
	slack, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HETZNER_PRICE_SLACK_PERCENT")), 64)
	if slack <= 0 {
		slack = 5
	}
	sync := 2 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("HETZNER_SYNC_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			sync = d
		}
	}
	base := strings.TrimSpace(os.Getenv("HETZNER_ROBOT_API_URL"))
	if base == "" {
		base = "https://robot-ws.your-server.de"
	}
	enabled := strings.EqualFold(strings.TrimSpace(os.Getenv("HETZNER_ROBOT_ENABLED")), "true")
	user := strings.TrimSpace(os.Getenv("HETZNER_ROBOT_USER"))
	pass := strings.TrimSpace(os.Getenv("HETZNER_ROBOT_PASSWORD"))
	if enabled && (user == "" || pass == "") {
		enabled = false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	eurRub := ResolveEurRub(ctx, fallback)

	extraMonthly, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HETZNER_EXTRA_IPV4_EUR")), 64)
	if extraMonthly <= 0 {
		extraMonthly = 1.70
	}
	extraSetup, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HETZNER_EXTRA_IPV4_SETUP_EUR")), 64)
	if extraSetup <= 0 {
		extraSetup = 4.90
	}
	extraMonthlyRub, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HETZNER_EXTRA_IPV4_MONTHLY_RUB")), 64)
	if extraMonthlyRub <= 0 {
		extraMonthlyRub = 250
	}
	extraSetupRub, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HETZNER_EXTRA_IPV4_SETUP_RUB")), 64)
	if extraSetupRub <= 0 {
		extraSetupRub = 500
	}
	extraMax, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("HETZNER_EXTRA_IPV4_MAX")))
	if extraMax <= 0 {
		extraMax = 4
	}
	if extraMax > 32 {
		extraMax = 32
	}

	return Config{
		User:                user,
		Password:            pass,
		BaseURL:             strings.TrimRight(base, "/"),
		Enabled:             enabled,
		MarkupPercent:       markup,
		EurRub:              eurRub,
		SyncInterval:        sync,
		PriceSlackPct:       slack,
		ExtraIPv4EUR:        extraMonthly,
		ExtraIPv4SetupEUR:   extraSetup,
		ExtraIPv4MonthlyRub: extraMonthlyRub,
		ExtraIPv4SetupRub:   extraSetupRub,
		ExtraIPv4Max:        extraMax,
	}
}

func (c Config) SellPriceRub(priceEUR float64) float64 {
	if priceEUR < 0 {
		priceEUR = 0
	}
	raw := priceEUR * c.EurRub * (1 + c.MarkupPercent/100)
	// Round UP to nearest 50 RUB (6523.8 → 6550).
	if raw <= 0 {
		return 0
	}
	return math.Ceil(raw/50) * 50
}

// ExtraIPv4UnitRub returns sell prices for one additional IPv4 (monthly + one-time setup).
// Uses fixed RUB prices (default 250 / 500) so catalog stays stable when FX moves.
func (c Config) ExtraIPv4UnitRub() (monthly, setup float64) {
	monthly = c.ExtraIPv4MonthlyRub
	if monthly <= 0 {
		monthly = c.SellPriceRub(c.ExtraIPv4EUR)
	}
	setup = c.ExtraIPv4SetupRub
	if setup <= 0 {
		setup = c.SellPriceRub(c.ExtraIPv4SetupEUR)
	}
	return monthly, setup
}

// ExtraIPv4OrderChargeRub is prepaid charge for qty IPs over periodMonths (with period discount on monthly part only).
func (c Config) ExtraIPv4OrderChargeRub(qty, periodMonths int, periodDiscount float64) float64 {
	if qty <= 0 || periodMonths <= 0 {
		return 0
	}
	monthly, setup := c.ExtraIPv4UnitRub()
	if periodDiscount < 0 {
		periodDiscount = 0
	}
	if periodDiscount > 0.5 {
		periodDiscount = 0.5
	}
	monthlyPart := monthly * float64(qty) * float64(periodMonths) * (1 - periodDiscount)
	setupPart := setup * float64(qty)
	return math.Round((monthlyPart+setupPart)*100) / 100
}

func (c Config) WithFreshFX(ctx context.Context) Config {
	fallback, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HETZNER_EUR_RUB")), 64)
	if fallback <= 0 {
		fallback = 100
	}
	c.EurRub = ResolveEurRub(ctx, fallback)
	return c
}
