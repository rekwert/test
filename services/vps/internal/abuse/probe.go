package abuse

import (
	"context"
	"fmt"
	"net"
	"time"
)

const smtpProbeTimeout = 3 * time.Second

// ProbeSMTPPorts returns listening mail ports on the given IP (from infrastructure).
func ProbeSMTPPorts(ip string, ports []int) []int {
	if ip == "" || len(ports) == 0 {
		return nil
	}
	open := make([]int, 0, len(ports))
	for _, port := range ports {
		addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
		conn, err := net.DialTimeout("tcp", addr, smtpProbeTimeout)
		if err != nil {
			continue
		}
		_ = conn.Close()
		open = append(open, port)
	}
	return open
}

// ProbeSMTPPortsContext is a context-aware wrapper for tests and worker use.
func ProbeSMTPPortsContext(ctx context.Context, ip string, ports []int) []int {
	if ctx.Err() != nil {
		return nil
	}
	return ProbeSMTPPorts(ip, ports)
}
