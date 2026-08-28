package handler

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

func consoleOriginAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" || origin == "null" {
		return true
	}

	allowed := consoleAllowedOrigins(r)
	oHost := originHost(origin)
	if oHost == "" {
		return false
	}
	for _, candidate := range allowed {
		if origin == candidate || strings.HasPrefix(origin, strings.TrimRight(candidate, "/")+"/") {
			return true
		}
		if h := originHost(candidate); h != "" && hostsEquivalent(oHost, h) {
			return true
		}
	}
	return strings.Contains(origin, "localhost")
}

func consoleAllowedOrigins(r *http.Request) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	if site := strings.TrimRight(strings.TrimSpace(os.Getenv("SITE_URL")), "/"); site != "" {
		add(site)
		if u, err := url.Parse(site); err == nil && u.Host != "" {
			add("https://" + u.Host)
			add("http://" + u.Host)
		}
	}
	if raw := os.Getenv("CORS_ORIGINS"); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			add(strings.TrimSpace(part))
		}
	}

	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host != "" {
		add("https://" + host)
		add("http://" + host)
		if i := strings.Index(host, ","); i > 0 {
			host = strings.TrimSpace(host[:i])
			add("https://" + host)
			add("http://" + host)
		}
	}
	return out
}

func originHost(raw string) string {
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return normalizeHost(u.Host)
	}
	return normalizeHost(raw)
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimSuffix(host, ":443")
	host = strings.TrimSuffix(host, ":80")
	if strings.HasPrefix(host, "www.") {
		host = host[4:]
	}
	return host
}

func hostsEquivalent(a, b string) bool {
	return normalizeHost(a) == normalizeHost(b)
}
