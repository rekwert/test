package prodenv

import (
	"log"
	"os"
	"strings"
)

func IsProduction() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) {
	case "production", "prod":
		return true
	default:
		return false
	}
}

var weakJWTSecrets = map[string]struct{}{
	"dev-secret":           {},
	"dev-secret-change-me": {},
	"changeme":             {},
}

const minJWTSecretLen = 32

func RequireJWTSecret(fallback string) string {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		if IsProduction() {
			log.Fatal("JWT_SECRET must be set when APP_ENV=production")
		}
		return fallback
	}
	if IsProduction() {
		if _, weak := weakJWTSecrets[strings.ToLower(secret)]; weak {
			log.Fatalf("JWT_SECRET must not use a development default in production")
		}
		if len(secret) < minJWTSecretLen {
			log.Fatalf("JWT_SECRET must be at least %d characters in production", minJWTSecretLen)
		}
	}
	return secret
}

// AssertPostgresDSNSecurity warns or fails when DB connections skip TLS in production.
func AssertPostgresDSNSecurity(dsn string) {
	if !IsProduction() {
		return
	}
	lower := strings.ToLower(dsn)
	if strings.Contains(lower, "sslmode=disable") || strings.Contains(lower, "sslmode=allow") {
		if strings.EqualFold(strings.TrimSpace(os.Getenv("POSTGRES_SSL_ALLOW_INSECURE")), "true") {
			log.Printf("WARN: POSTGRES_DSN uses insecure sslmode in production (POSTGRES_SSL_ALLOW_INSECURE=true)")
			return
		}
		log.Fatal("POSTGRES_DSN must use sslmode=require (or verify-full) in production; set POSTGRES_SSL_ALLOW_INSECURE=true only during migration")
	}
}

// AssertBackGatewayBind checks that the gateway is not unintentionally public.
func AssertBackGatewayBind() {
	if !IsProduction() {
		return
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("BACK_GATEWAY_PUBLIC_OK")), "true") {
		return
	}
	bind := strings.TrimSpace(os.Getenv("BACK_BIND_IP"))
	if bind == "" || bind == "0.0.0.0" {
		log.Fatal("BACK_BIND_IP must not be 0.0.0.0 in production without UFW; run infra/scripts/harden-back-ufw.sh then set BACK_GATEWAY_PUBLIC_OK=true")
	}
}

// AssertBillingNotMock exits when mock billing is enabled in production.
func AssertBillingNotMock(mock bool) {
	if IsProduction() && mock {
		log.Fatal("BILLING_MOCK must be false when APP_ENV=production")
	}
}

// AssertOpenStackTLS exits when TLS verification is disabled in production.
func AssertOpenStackTLS(insecure bool) {
	if IsProduction() && insecure {
		log.Fatal("OPENSTACK_INSECURE_TLS must be false when APP_ENV=production")
	}
}

var weakInternalTokens = map[string]struct{}{
	"dev-internal-token": {},
	"change_me":          {},
	"change_me_internal_token": {},
}

func RequireFieldEncryptionKey(envKey string) {
	if !IsProduction() {
		return
	}
	if strings.TrimSpace(os.Getenv(envKey)) == "" {
		log.Fatalf("%s must be set when APP_ENV=production", envKey)
	}
}

func RequireInternalToken(envKey, label string) string {
	token := strings.TrimSpace(os.Getenv(envKey))
	if token == "" {
		if IsProduction() {
			log.Fatalf("%s must be set when APP_ENV=production", envKey)
		}
		return ""
	}
	if IsProduction() {
		if _, weak := weakInternalTokens[strings.ToLower(token)]; weak {
			log.Fatalf("%s must not use a development default in production (%s)", envKey, label)
		}
	}
	return token
}
