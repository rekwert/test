package smtpblock

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Ports blocked by default for guest outbound SMTP / spam bypass.
var Ports = []int{25, 2525}

// SetGuestAllowed adds or removes the guest IP on the hypervisor smtp_allow ipset
// (and ensures base FORWARD drop rules for ports 25/2525 exist).
func SetGuestAllowed(ctx context.Context, hvHost, guestIP string, allow bool) error {
	hvHost = strings.TrimSpace(hvHost)
	guestIP = strings.TrimSpace(guestIP)
	if hvHost == "" {
		return fmt.Errorf("smtpblock: empty hypervisor host")
	}
	if net.ParseIP(guestIP) == nil || strings.Contains(guestIP, ":") {
		return fmt.Errorf("smtpblock: invalid guest ipv4 %q", guestIP)
	}
	cfg, err := hvSSHConfigForHost(hvHost)
	if err != nil {
		return err
	}
	script := ensureBaseScript()
	if allow {
		script += fmt.Sprintf("\nipset add smtp_allow %s -exist\n", guestIP)
	} else {
		script += fmt.Sprintf("\nipset del smtp_allow %s 2>/dev/null || true\n", guestIP)
	}
	return execOnHV(ctx, hvHost, cfg, script)
}

func execOnHV(ctx context.Context, hvHost string, cfg *hvSSHSettings, script string) error {
	if jump := hvJumpHost(hvHost); jump != "" {
		return execViaJump(ctx, jump, hvHost, cfg, script)
	}
	conn, err := dialHV(hvHost, cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	return runRemote(ctx, conn, script)
}

// hvJumpHost returns an SSH bastion for hypervisors reachable only via ops jump host.
func hvJumpHost(hvHost string) string {
	switch strings.TrimSpace(hvHost) {
	case "212.102.227.7":
		return firstEnv("HV_SSH_JUMP_HOST", "VIRTFUSION_HV_SSH_JUMP_HOST", "VIRTFUSION_CTRL_SSH_HOST")
	default:
		return ""
	}
}

func execViaJump(ctx context.Context, jumpHost, hvHost string, cfg *hvSSHSettings, script string) error {
	jumpCfg, err := hvSSHConfigForHost(jumpHost)
	if err != nil {
		return fmt.Errorf("smtpblock: jump host %s: %w", jumpHost, err)
	}
	conn, err := dialHV(jumpHost, jumpCfg)
	if err != nil {
		return fmt.Errorf("smtpblock: jump dial %s: %w", jumpHost, err)
	}
	defer conn.Close()

	remote := fmt.Sprintf(
		"ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o ConnectTimeout=15 -p %s %s@%s bash -lc %s",
		cfg.port, cfg.user, hvHost, shellQuote(script),
	)
	return runRemote(ctx, conn, remote)
}

// EnsureBaseRules installs default DROP for outbound 25/2525 plus smtp_allow exceptions.
func EnsureBaseRules(ctx context.Context, hvHost string) error {
	hvHost = strings.TrimSpace(hvHost)
	if hvHost == "" {
		return fmt.Errorf("smtpblock: empty hypervisor host")
	}
	cfg, err := hvSSHConfigForHost(hvHost)
	if err != nil {
		return err
	}
	return execOnHV(ctx, hvHost, cfg, ensureBaseScript())
}

// hvPasswordCandidates returns passwords to try for a hypervisor host.
// Per-region ops passwords (DE/FI/GB) are optional; ctrl password is the fallback.
func hvPasswordCandidates(hvHost string) []string {
	host := strings.TrimSpace(hvHost)
	seen := map[string]struct{}{}
	var out []string
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
	for _, envKey := range hvPasswordEnvKeys(host) {
		add(os.Getenv(envKey))
	}
	add(firstEnv("HV_SSH_PASSWORD", "VIRTFUSION_HV_SSH_PASSWORD", "VIRTFUSION_CTRL_SSH_PASSWORD"))
	return out
}

func hvPasswordEnvKeys(host string) []string {
	switch host {
	case "212.102.227.6":
		return []string{"DE_SSH_PASS", "DE_SSH_PASSWORD"}
	case "212.102.227.7":
		return []string{"DE_MID_SSH_PASS", "DE_MID_SSH_PASSWORD", "DE_SSH_PASS", "DE_SSH_PASSWORD"}
	case "95.216.1.155":
		return []string{"FI_SSH_PASS", "FI_SSH_PASSWORD"}
	case "212.108.83.47":
		return []string{"GB_SSH_PASS", "GB_SSH_PASSWORD"}
	case "66.248.206.14":
		return []string{"NL_SSH_PASS", "NL_SSH_PASSWORD"}
	default:
		return nil
	}
}

func ensureBaseScript() string {
	return strings.TrimSpace(`
set -e
command -v ipset >/dev/null 2>&1 || { apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ipset iptables >/dev/null; }
ipset create smtp_allow hash:ip family inet -exist
# Allow listed guests first.
iptables -C FORWARD -m set --match-set smtp_allow src -p tcp -m multiport --dports 25,2525 -j ACCEPT 2>/dev/null || \
  iptables -I FORWARD 1 -m set --match-set smtp_allow src -p tcp -m multiport --dports 25,2525 -j ACCEPT
# Default deny outbound SMTP from guests.
iptables -C FORWARD -p tcp -m multiport --dports 25,2525 -j DROP 2>/dev/null || \
  iptables -A FORWARD -p tcp -m multiport --dports 25,2525 -j DROP
`) + "\n"
}

func runRemote(ctx context.Context, conn *ssh.Client, script string) error {
	session, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("smtpblock: session: %w", err)
	}
	defer session.Close()

	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, runErr := session.CombinedOutput("bash -lc " + shellQuote(script))
		done <- result{out: out, err: runErr}
	}()

	select {
	case <-ctx.Done():
		_ = session.Close()
		return ctx.Err()
	case res := <-done:
		out := strings.TrimSpace(string(res.out))
		if res.err != nil {
			if out == "" {
				return fmt.Errorf("smtpblock: remote: %w", res.err)
			}
			return fmt.Errorf("smtpblock: remote: %w: %s", res.err, out)
		}
		return nil
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

type hvSSHSettings struct {
	port      string
	user      string
	keySigner ssh.Signer
	passwords []string
}

func dialHV(hvHost string, cfg *hvSSHSettings) (*ssh.Client, error) {
	addr := net.JoinHostPort(hvHost, cfg.port)
	base := &ssh.ClientConfig{
		User:            cfg.user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // ops-managed hypervisors
		Timeout:         25 * time.Second,
	}
	if cfg.keySigner != nil {
		client := *base
		client.Auth = []ssh.AuthMethod{ssh.PublicKeys(cfg.keySigner)}
		if conn, err := ssh.Dial("tcp", addr, &client); err == nil {
			return conn, nil
		}
	}
	var lastErr error
	for _, pass := range cfg.passwords {
		client := *base
		client.Auth = []ssh.AuthMethod{ssh.Password(pass)}
		conn, err := ssh.Dial("tcp", addr, &client)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, fmt.Errorf("smtpblock: ssh dial %s: %w", addr, lastErr)
	}
	return nil, fmt.Errorf("smtpblock: ssh dial %s: no credentials", addr)
}

func hvSSHConfigForHost(hvHost string) (*hvSSHSettings, error) {
	user := firstEnv("HV_SSH_USER", "VIRTFUSION_HV_SSH_USER", "VIRTFUSION_CTRL_SSH_USER")
	if user == "" {
		user = "root"
	}
	port := firstEnv("HV_SSH_PORT", "VIRTFUSION_HV_SSH_PORT", "VIRTFUSION_CTRL_SSH_PORT")
	if port == "" {
		port = "22"
	}

	cfg := &hvSSHSettings{port: port, user: user}
	if key := firstEnv("HV_SSH_PRIVATE_KEY", "VIRTFUSION_HV_SSH_PRIVATE_KEY", "VIRTFUSION_CTRL_SSH_PRIVATE_KEY", "VPS_OPS_SSH_PRIVATE_KEY"); key != "" {
		key = normalizeSSHPrivateKey(key)
		signer, err := ssh.ParsePrivateKey([]byte(key))
		if err != nil {
			return nil, fmt.Errorf("smtpblock: hv ssh private key: %w", err)
		}
		cfg.keySigner = signer
	}
	cfg.passwords = hvPasswordCandidates(hvHost)
	if cfg.keySigner == nil && len(cfg.passwords) == 0 {
		return nil, fmt.Errorf("smtpblock: hv ssh credentials missing (set HV_SSH_* or legacy VIRTFUSION_* SSH env)")
	}
	return cfg, nil
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func normalizeSSHPrivateKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "BEGIN") {
		return raw
	}
	return "-----BEGIN OPENSSH PRIVATE KEY-----\n" + raw + "\n-----END OPENSSH PRIVATE KEY-----\n"
}
