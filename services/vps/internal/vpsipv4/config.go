package vpsipv4

import (
	"os"
	"strconv"
	"strings"
)

// Config holds sell price for additional VPS IPv4 addresses (VirtFusion pool).
type Config struct {
	PriceRub float64
	MaxQty   int
}

func Load() Config {
	price, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("VPS_EXTRA_IPV4_RUB")), 64)
	if price <= 0 {
		// Legacy env names (monthly only — setup is not charged for VPS rent).
		price, _ = strconv.ParseFloat(strings.TrimSpace(os.Getenv("VPS_EXTRA_IPV4_MONTHLY_RUB")), 64)
	}
	if price <= 0 {
		price = 250
	}
	maxQty, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("VPS_EXTRA_IPV4_MAX")))
	if maxQty <= 0 {
		maxQty = 5
	}
	return Config{
		PriceRub: price,
		MaxQty:   maxQty,
	}
}

// OrderChargeRub is the balance debit for qty extra IPs (flat price per address).
func (c Config) OrderChargeRub(qty int) float64 {
	if qty <= 0 {
		return 0
	}
	return float64(qty) * c.PriceRub
}
