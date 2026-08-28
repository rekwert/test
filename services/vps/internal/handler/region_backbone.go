package handler

import (
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

const (
	backboneCacheTTL = 60 * time.Second
	backboneDialTO   = 1500 * time.Millisecond
)

type backboneCache struct {
	mu   sync.Mutex
	at   time.Time
	data map[string]int64
}

var regionBackboneCache backboneCache

func backboneProbePort() string {
	port := strings.TrimSpace(os.Getenv("REGION_BACKBONE_PROBE_PORT"))
	if port == "" {
		return "8892"
	}
	return port
}

func measureTCPMs(host, port string) (int64, bool) {
	host = strings.TrimSpace(host)
	if host == "" {
		return 0, false
	}
	addr := net.JoinHostPort(host, port)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, backboneDialTO)
	if err != nil {
		return 0, false
	}
	_ = conn.Close()
	ms := time.Since(start).Milliseconds()
	if ms <= 0 {
		ms = 1
	}
	return ms, true
}

func backboneLatencyMatrix(regions []store.RegionRow) map[string]int64 {
	regionBackboneCache.mu.Lock()
	defer regionBackboneCache.mu.Unlock()

	if regionBackboneCache.data != nil && time.Since(regionBackboneCache.at) <= backboneCacheTTL {
		return cloneBackboneMap(regionBackboneCache.data)
	}

	port := backboneProbePort()
	out := make(map[string]int64, len(regions))
	for _, reg := range regions {
		code := strings.ToLower(strings.TrimSpace(reg.Code))
		if code == "" {
			continue
		}
		if ms, ok := measureTCPMs(reg.ProbeHost, port); ok {
			out[code] = ms
		}
	}

	regionBackboneCache.at = time.Now()
	regionBackboneCache.data = out
	return cloneBackboneMap(out)
}

func cloneBackboneMap(in map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func backboneMsForRegion(matrix map[string]int64, code string) (int64, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return 0, false
	}
	ms, ok := matrix[code]
	if !ok || ms <= 0 {
		return 0, false
	}
	if ms > 1200 {
		return 0, false
	}
	return ms, true
}
