package clientip

import (
	"net"
	"net/http"
	"os"
	"strings"
)

// ClientIP returns the client IP, honoring X-Forwarded-For only when RemoteAddr
// is a configured trusted proxy (TRUSTED_PROXY_IPS, comma-separated).
func ClientIP(r *http.Request) string {
	if trustedProxyIP(r.RemoteAddr) || remoteFromPrivateNetwork(r.RemoteAddr) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
		if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
			return xri
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func trustedProxyIP(remoteAddr string) bool {
	raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXY_IPS"))
	if raw == "" {
		return false
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	for _, p := range strings.Split(raw, ",") {
		if strings.TrimSpace(p) == host {
			return true
		}
	}
	return false
}
