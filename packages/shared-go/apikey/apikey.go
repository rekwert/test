package apikey

import (
	"strings"
)

const BearerPrefix = "ch_live_"

func TokenFromRequest(authHeader, apiKeyHeader string) string {
	if v := strings.TrimSpace(apiKeyHeader); v != "" {
		return v
	}
	raw := strings.TrimSpace(authHeader)
	const prefix = "Bearer "
	if !strings.HasPrefix(raw, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(raw, prefix))
}

func IsAPIKeyToken(token string) bool {
	return strings.HasPrefix(strings.TrimSpace(token), BearerPrefix)
}

func HasScope(scopes []string, need string) bool {
	for _, s := range scopes {
		if s == need {
			return true
		}
	}
	return false
}