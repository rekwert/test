package hostkey

import (
	"context"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hetznerrobot"
)

type Config struct {
	Token         string
	BaseURL       string
	Enabled       bool
	MarkupPercent float64
	EurRub        float64
	SyncInterval  time.Duration
	PriceSlackPct float64
	ExtraIPv4Max  int
}

func LoadConfig() Config {
	markup, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HOSTKEY_MARKUP_PERCENT")), 64)
	if markup <= 0 {
		markup = 10
	}
	if markup < 0 {
		markup = 0
	}
	slack, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HOSTKEY_PRICE_SLACK_PERCENT")), 64)
	if slack <= 0 {
		slack = 5
	}
	sync := 5 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("HOSTKEY_SYNC_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			sync = d
		}
	}
	base := strings.TrimSpace(os.Getenv("HOSTKEY_INVAPI_URL"))
	if base == "" {
		base = "https://invapi.hostkey.com"
	}
	token := strings.TrimSpace(os.Getenv("HOSTKEY_API_TOKEN"))
	enabled := strings.EqualFold(strings.TrimSpace(os.Getenv("HOSTKEY_ENABLED")), "true")
	if enabled && token == "" {
		enabled = false
	}

	fallback, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HOSTKEY_EUR_RUB")), 64)
	if fallback <= 0 {
		fallback, _ = strconv.ParseFloat(strings.TrimSpace(os.Getenv("HETZNER_EUR_RUB")), 64)
	}
	if fallback <= 0 {
		fallback = 100
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	eurRub := hetznerrobot.ResolveEurRub(ctx, fallback)

	extraMax, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("HOSTKEY_EXTRA_IPV4_MAX")))
	if extraMax <= 0 {
		extraMax = 4
	}
	if extraMax > 32 {
		extraMax = 32
	}

	return Config{
		Token:         token,
		BaseURL:       strings.TrimRight(base, "/"),
		Enabled:       enabled,
		MarkupPercent: markup,
		EurRub:        eurRub,
		SyncInterval:  sync,
		PriceSlackPct: slack,
		ExtraIPv4Max:  extraMax,
	}
}

func (c Config) SellPriceRub(priceEUR float64) float64 {
	if priceEUR <= 0 {
		return 0
	}
	raw := priceEUR * c.EurRub * (1 + c.MarkupPercent/100)
	return math.Round(raw*100) / 100
}

// SellPriceFromNative applies markup to Hostkey list price in RUB (monthly_ru / RUR).
func (c Config) SellPriceFromNative(priceRUB float64) float64 {
	if priceRUB <= 0 {
		return 0
	}
	raw := priceRUB * (1 + c.MarkupPercent/100)
	return math.Round(raw*100) / 100
}

// SellPrice prefers native RUB from Invapi; EUR→RUB only when RUB is missing.
func (c Config) SellPrice(priceEUR, priceRUB float64) float64 {
	if priceRUB > 0 {
		return c.SellPriceFromNative(priceRUB)
	}
	return c.SellPriceRub(priceEUR)
}

func (c Config) WithFreshFX(ctx context.Context) Config {
	fallback, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HOSTKEY_EUR_RUB")), 64)
	if fallback <= 0 {
		fallback, _ = strconv.ParseFloat(strings.TrimSpace(os.Getenv("HETZNER_EUR_RUB")), 64)
	}
	if fallback <= 0 {
		fallback = 100
	}
	c.EurRub = hetznerrobot.ResolveEurRub(ctx, fallback)
	return c
}
