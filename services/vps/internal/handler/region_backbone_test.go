package handler

import (
	"net"
	"testing"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

func TestMeasureTCPMs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		_ = conn.Close()
	}()

	ms, ok := measureTCPMs("127.0.0.1", port)
	if !ok || ms <= 0 {
		t.Fatalf("expected positive latency, got ok=%v ms=%d", ok, ms)
	}
}

func TestBackboneLatencyMatrixCaches(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	t.Setenv("REGION_BACKBONE_PROBE_PORT", port)

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	host, _, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	regions := []store.RegionRow{{Code: "nl", ProbeHost: host}}
	regionBackboneCache = backboneCache{}

	first := backboneLatencyMatrix(regions)
	if got := first["nl"]; got <= 0 {
		t.Fatalf("expected nl backbone ms, got %d", got)
	}

	regionBackboneCache.at = time.Now().Add(-2 * backboneCacheTTL)
	second := backboneLatencyMatrix(regions)
	if got := second["nl"]; got <= 0 {
		t.Fatalf("expected refreshed nl backbone ms, got %d", got)
	}
}
