package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
)

type Config struct {
	Port        string
	Mock        bool
	JWTSecret   string
	PostgresDSN string

	TBankTerminalKey     string
	TBankPassword        string
	TBankAPIURL          string
	TBankNotificationURL string
	TBankSuccessURL      string
	TBankFailURL         string
	TBankEnabled         bool
	TBankTaxation        string
	TBankReceiptTax      string
	TBankReceiptItemName string

	HeleketEnabled      bool
	HeleketMerchantID   string
	HeleketAPIKey       string
	HeleketAPIURL       string
	HeleketCallbackURL  string
	HeleketSuccessURL   string
	HeleketReturnURL    string
	HeleketCurrency     string

	RobokassaEnabled     bool
	RobokassaMerchantLogin string
	RobokassaPassword1     string
	RobokassaPassword2     string
	RobokassaTestPassword1 string
	RobokassaTestPassword2 string
	RobokassaTestMode      bool
	RobokassaSuccessURL  string
	RobokassaFailURL     string
}

func Load() Config {
	return Config{
		Port:        env("PORT", "8002"),
		Mock:        boolEnv("BILLING_MOCK", false),
		JWTSecret:   prodenv.RequireJWTSecret("dev-secret"),
		PostgresDSN: env("POSTGRES_DSN", ""),

		TBankTerminalKey:     env("TBANK_TERMINAL_KEY", ""),
		TBankPassword:        env("TBANK_PASSWORD", ""),
		TBankAPIURL:          env("TBANK_API_URL", "https://securepay.tinkoff.ru/v2"),
		TBankNotificationURL: env("TBANK_NOTIFICATION_URL", ""),
		TBankSuccessURL:      env("TBANK_SUCCESS_URL", ""),
		TBankFailURL:         env("TBANK_FAIL_URL", ""),
		TBankEnabled:         boolEnv("TBANK_ENABLED", false),
		TBankTaxation:        env("TBANK_TAXATION", "usn_income"),
		TBankReceiptTax:      env("TBANK_RECEIPT_TAX", "none"),
		TBankReceiptItemName: env("TBANK_RECEIPT_ITEM_NAME", "Пополнение баланса VPS"),

		HeleketEnabled:     boolEnv("HELEKET_ENABLED", false),
		HeleketMerchantID:  env("HELEKET_MERCHANT_ID", ""),
		HeleketAPIKey:      env("HELEKET_API_KEY", ""),
		HeleketAPIURL:      env("HELEKET_API_URL", "https://api.heleket.com/v1"),
		HeleketCallbackURL: env("HELEKET_CALLBACK_URL", ""),
		HeleketSuccessURL:  env("HELEKET_SUCCESS_URL", ""),
		HeleketReturnURL:   env("HELEKET_RETURN_URL", ""),
		HeleketCurrency:    env("HELEKET_CURRENCY", "RUB"),

		RobokassaEnabled:       boolEnv("ROBOKASSA_ENABLED", false),
		RobokassaMerchantLogin: env("ROBOKASSA_MERCHANT_LOGIN", ""),
		RobokassaPassword1:       env("ROBOKASSA_PASSWORD1", ""),
		RobokassaPassword2:       env("ROBOKASSA_PASSWORD2", ""),
		RobokassaTestPassword1:   env("ROBOKASSA_TEST_PASSWORD1", ""),
		RobokassaTestPassword2:   env("ROBOKASSA_TEST_PASSWORD2", ""),
		RobokassaTestMode:        boolEnv("ROBOKASSA_TEST_MODE", false),
		RobokassaSuccessURL:    env("ROBOKASSA_SUCCESS_URL", ""),
		RobokassaFailURL:       env("ROBOKASSA_FAIL_URL", ""),
	}
}

func (c Config) HeleketReady() bool {
	return c.HeleketEnabled && c.HeleketMerchantID != "" && c.HeleketAPIKey != ""
}

func (c Config) TBankReady() bool {
	return c.TBankEnabled && c.TBankTerminalKey != "" && c.TBankPassword != ""
}

func (c Config) RobokassaReady() bool {
	if !c.RobokassaEnabled || c.RobokassaMerchantLogin == "" {
		return false
	}
	if c.RobokassaTestMode {
		p1 := c.RobokassaTestPassword1
		if p1 == "" {
			p1 = c.RobokassaPassword1
		}
		p2 := c.RobokassaTestPassword2
		if p2 == "" {
			p2 = c.RobokassaPassword2
		}
		return p1 != "" && p2 != ""
	}
	return c.RobokassaPassword1 != "" && c.RobokassaPassword2 != ""
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func boolEnv(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "1" || v == "true" || v == "yes" {
		return true
	}
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
