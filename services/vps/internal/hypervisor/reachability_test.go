package hypervisor

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestProbeTCP_invalidHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if ProbeTCP(ctx, "203.0.113.1", DefaultHypervisorPort) {
		t.Fatal("expected unreachable RFC5737 TEST-NET-3 host to fail")
	}
}

func TestProbeTCP_emptyHost(t *testing.T) {
	ctx := context.Background()
	if ProbeTCP(ctx, "", DefaultHypervisorPort) {
		t.Fatal("expected empty host to fail")
	}
}

func TestProbeTCP_localListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !ProbeTCP(ctx, "127.0.0.1", port) {
		t.Fatal("expected local listener to be reachable")
	}
}
