package clientip

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Endpoint holds client IP and ports for compliance logging (149-FZ / PP 1526).
// ClientPort is the user's ephemeral source port when known (often NULL behind Traefik without PROXY protocol).
// ServerPort is the public service port the client connected to (typically 443).
type Endpoint struct {
	IP         string
	ClientPort *int
	ServerPort *int
}

func ClientEndpoint(r *http.Request) Endpoint {
	ep := Endpoint{IP: ClientIP(r)}

	if cp := clientPortFromRequest(r); cp != nil {
		ep.ClientPort = cp
	}
	ep.ServerPort = serverPortFromRequest(r)

	return ep
}

func clientPortFromRequest(r *http.Request) *int {
	for _, h := range []string{"X-Client-Port", "X-Real-Port"} {
		if p := parsePort(r.Header.Get(h)); p != nil {
			return p
		}
	}
	if p := clientPortFromForwarded(r.Header.Get("Forwarded")); p != nil {
		return p
	}
	if trustedProxyIP(r.RemoteAddr) || remoteFromPrivateNetwork(r.RemoteAddr) {
		return nil
	}
	if _, portStr, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return parsePort(portStr)
	}
	return nil
}

func serverPortFromRequest(r *http.Request) *int {
	if p := parsePort(r.Header.Get("X-Forwarded-Port")); p != nil {
		return p
	}
	if r.TLS != nil {
		p := 443
		return &p
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		p := 443
		return &p
	}
	if _, portStr, err := net.SplitHostPort(r.Host); err == nil {
		return parsePort(portStr)
	}
	if p := parsePort(strings.TrimSpace(osPublicPort())); p != nil {
		return p
	}
	return nil
}

func osPublicPort() string {
	return strings.TrimSpace(os.Getenv("GATEWAY_PUBLIC_PORT"))
}

func parsePort(raw string) *int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	p, err := strconv.Atoi(raw)
	if err != nil || p <= 0 || p > 65535 {
		return nil
	}
	return &p
}

// clientPortFromForwarded parses RFC 7239 Forwarded, e.g. for=192.0.2.60:57005;proto=https
func clientPortFromForwarded(raw string) *int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		for _, seg := range strings.Split(part, ";") {
			seg = strings.TrimSpace(seg)
			if !strings.HasPrefix(strings.ToLower(seg), "for=") {
				continue
			}
			val := strings.TrimPrefix(seg, "for=")
			val = strings.TrimPrefix(val, "For=")
			val = strings.Trim(val, `"`)
			if strings.HasPrefix(val, "[") {
				if idx := strings.LastIndex(val, "]:"); idx >= 0 {
					return parsePort(val[idx+2:])
				}
				continue
			}
			if host, portStr, err := net.SplitHostPort(val); err == nil && host != "" {
				return parsePort(portStr)
			}
		}
	}
	return nil
}

func remoteFromPrivateNetwork(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback()
}

// SetForwardHeaders sets X-Forwarded-For (appends), X-Client-Port, X-Forwarded-Port for upstream services.
func SetForwardHeaders(r *http.Request, ep Endpoint, defaultPublicPort int) {
	clientIP := strings.TrimSpace(ep.IP)
	if clientIP == "" {
		clientIP = ClientIP(r)
	}
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	switch {
	case xff == "":
		r.Header.Set("X-Forwarded-For", clientIP)
	case !strings.Contains(xff, clientIP):
		r.Header.Set("X-Forwarded-For", clientIP+", "+xff)
	}

	if ep.ClientPort != nil {
		r.Header.Set("X-Client-Port", strconv.Itoa(*ep.ClientPort))
	} else if !trustedProxyIP(r.RemoteAddr) && !remoteFromPrivateNetwork(r.RemoteAddr) {
		if _, portStr, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			if p := parsePort(portStr); p != nil {
				r.Header.Set("X-Client-Port", strconv.Itoa(*p))
			}
		}
	}

	serverPort := ep.ServerPort
	if serverPort == nil && defaultPublicPort > 0 {
		serverPort = &defaultPublicPort
	}
	if serverPort != nil {
		r.Header.Set("X-Forwarded-Port", strconv.Itoa(*serverPort))
	}
}
