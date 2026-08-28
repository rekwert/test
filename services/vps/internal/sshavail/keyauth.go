package sshavail

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// GuestSSHAuth carries either password or private-key auth for guest automation.
type GuestSSHAuth struct {
	Password string
	Signer   ssh.Signer
}

func (a GuestSSHAuth) empty() bool {
	return strings.TrimSpace(a.Password) == "" && a.Signer == nil
}

func dialRootAuth(ctx context.Context, ip string, auth GuestSSHAuth) (*ssh.Client, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" || auth.empty() {
		return nil, fmt.Errorf("sshavail: missing ip or auth")
	}
	methods := make([]ssh.AuthMethod, 0, 2)
	if auth.Signer != nil {
		methods = append(methods, ssh.PublicKeys(auth.Signer))
	}
	if p := strings.TrimSpace(auth.Password); p != "" {
		methods = append(methods, ssh.Password(p))
	}
	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            methods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         12 * time.Second,
	}
	dialer := &net.Dialer{Timeout: 12 * time.Second}
	addr := net.JoinHostPort(ip, "22")
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("sshavail: dial %s: %w", addr, err)
	}
	deadline, ok := ctx.Deadline()
	if ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("sshavail: auth failed: %w", err)
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), nil
}

// CheckRootAuth verifies root SSH using password and/or private key.
func CheckRootAuth(ctx context.Context, ip string, auth GuestSSHAuth) error {
	client, err := dialRootAuth(ctx, ip, auth)
	if err != nil {
		return err
	}
	defer client.Close()
	return run(client, "true")
}

// DialAnyRootAuth connects via the first reachable IP in candidates.
func DialAnyRootAuth(ctx context.Context, candidates []string, auth GuestSSHAuth) (string, error) {
	for _, ip := range candidates {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if err := CheckRootAuth(ctx, ip, auth); err == nil {
			return ip, nil
		}
	}
	return "", fmt.Errorf("sshavail: no reachable ip for root ssh")
}

// ReconfigureStaticIPv4Auth updates guest network config using password or key auth.
func ReconfigureStaticIPv4Auth(ctx context.Context, dialIP string, auth GuestSSHAuth, newIP, gateway string, prefix int) error {
	dialIP = strings.TrimSpace(dialIP)
	newIP = strings.TrimSpace(newIP)
	gateway = strings.TrimSpace(gateway)
	if dialIP == "" || newIP == "" || auth.empty() {
		return fmt.Errorf("sshavail: dialIP, newIP and auth required")
	}
	if prefix <= 0 || prefix > 32 {
		prefix = 24
	}
	if gateway == "" {
		parts := strings.Split(newIP, ".")
		if len(parts) == 4 {
			gateway = parts[0] + "." + parts[1] + "." + parts[2] + ".1"
		}
	}
	client, err := dialRootAuth(ctx, dialIP, auth)
	if err != nil {
		return err
	}
	defer client.Close()
	script := buildStaticIPScript(newIP, gateway, prefix)
	if err := run(client, script); err != nil {
		return fmt.Errorf("sshavail: reconfigure ipv4: %w", err)
	}
	return nil
}

// RunScriptAuth executes a remote bash script as root.
func RunScriptAuth(ctx context.Context, ip string, auth GuestSSHAuth, script string) error {
	client, err := dialRootAuth(ctx, ip, auth)
	if err != nil {
		return err
	}
	defer client.Close()
	return run(client, script)
}
