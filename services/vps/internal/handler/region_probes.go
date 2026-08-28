package handler

import (
	"os"
	"strings"
)

func regionProbeURL(code, probeHost string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return ""
	}
	if probes := parseRegionProbeMap(os.Getenv("REGION_PROBE_URLS")); probes != nil {
		if url := probes[code]; url != "" {
			return url
		}
	}
	// Do not expose hypervisor vf_ip as probe_url — browsers hit TLS errors and
	// report multi-second "latency" that is identical for all users.
	_ = probeHost
	return ""
}

func parseRegionProbeMap(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}
