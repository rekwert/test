package hypervisor

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"
)

const DefaultHypervisorPort = 8892

func ProbeTCP(ctx context.Context, host string, port int) bool {
	host = strings.TrimSpace(host)
	if host == "" || port <= 0 {
		return false
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
