package hetznerrobot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultCBRURL = "https://www.cbr-xml-daily.ru/daily_json.js"

var (
	fxMu        sync.RWMutex
	fxEurRub    float64
	fxFetchedAt time.Time
	fxSource    string
)

type cbrDailyJSON struct {
	Valute map[string]struct {
		Nominal float64 `json:"Nominal"`
		Value   float64 `json:"Value"`
	} `json:"Valute"`
}

func fxSourceFromEnv() string {
	src := strings.ToLower(strings.TrimSpace(os.Getenv("HETZNER_FX_SOURCE")))
	if src == "" {
		return "cbr"
	}
	return src
}

func fxCacheTTLFromEnv() time.Duration {
	ttl := time.Hour
	if raw := strings.TrimSpace(os.Getenv("HETZNER_FX_CACHE_TTL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			ttl = d
		}
	}
	return ttl
}

func fxSpreadFromEnv() float64 {
	spread, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("HETZNER_FX_SPREAD_PERCENT")), 64)
	if spread < 0 {
		return 0
	}
	return spread
}

func cbrURLFromEnv() string {
	u := strings.TrimSpace(os.Getenv("HETZNER_FX_CBR_URL"))
	if u == "" {
		return defaultCBRURL
	}
	return u
}

func FetchCBREurRub(ctx context.Context) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cbrURLFromEnv(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("cbr http %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var payload cbrDailyJSON
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, err
	}
	eur, ok := payload.Valute["EUR"]
	if !ok || eur.Value <= 0 {
		return 0, fmt.Errorf("cbr: EUR rate missing")
	}
	nominal := eur.Nominal
	if nominal <= 0 {
		nominal = 1
	}
	rate := eur.Value / nominal
	spread := fxSpreadFromEnv()
	if spread > 0 {
		rate *= 1 + spread/100
	}
	return rate, nil
}

func CachedEurRub() (rate float64, at time.Time, source string) {
	fxMu.RLock()
	defer fxMu.RUnlock()
	return fxEurRub, fxFetchedAt, fxSource
}

func RefreshEurRub(ctx context.Context, fallback float64) (float64, error) {
	if fallback <= 0 {
		fallback = 100
	}
	src := fxSourceFromEnv()
	if src == "fixed" {
		setCachedEurRub(fallback, "fixed")
		return fallback, nil
	}

	fxMu.RLock()
	age := time.Since(fxFetchedAt)
	cached := fxEurRub
	fxMu.RUnlock()
	if cached > 0 && age < fxCacheTTLFromEnv() {
		return cached, nil
	}

	rate, err := FetchCBREurRub(ctx)
	if err != nil {
		if cached > 0 {
			return cached, fmt.Errorf("cbr refresh failed, using cache %.4f: %w", cached, err)
		}
		setCachedEurRub(fallback, "fallback")
		return fallback, fmt.Errorf("cbr refresh failed, using fallback %.4f: %w", fallback, err)
	}
	setCachedEurRub(rate, "cbr")
	return rate, nil
}

func setCachedEurRub(rate float64, source string) {
	fxMu.Lock()
	fxEurRub = rate
	fxFetchedAt = time.Now()
	fxSource = source
	fxMu.Unlock()
}

func ResolveEurRub(ctx context.Context, fallback float64) float64 {
	rate, err := RefreshEurRub(ctx, fallback)
	if err != nil {
		_ = err
	}
	return rate
}
