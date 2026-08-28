package sshavail

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// HostKeyMaterial is OpenSSH host key pair text (private + public).
type HostKeyMaterial struct {
	Ed25519Private string
	Ed25519Public  string
	ECDSAPrivate   string
	ECDSAPublic    string
}

// CheckRootPassword dials IP:22 and authenticates as root with password.
func CheckRootPassword(ctx context.Context, ip, password string) error {
	client, err := dialRoot(ctx, ip, password)
	if err != nil {
		return err
	}
	defer client.Close()
	return run(client, "true")
}

// ReadHostKeys fetches current SSH host keys from the guest.
func ReadHostKeys(ctx context.Context, ip, password string) (*HostKeyMaterial, error) {
	client, err := dialRoot(ctx, ip, password)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	edPriv, err := runOut(client, "cat /etc/ssh/ssh_host_ed25519_key")
	if err != nil {
		return nil, fmt.Errorf("read ed25519 private: %w", err)
	}
	edPub, err := runOut(client, "cat /etc/ssh/ssh_host_ed25519_key.pub")
	if err != nil {
		return nil, fmt.Errorf("read ed25519 public: %w", err)
	}
	out := &HostKeyMaterial{
		Ed25519Private: strings.TrimSpace(edPriv) + "\n",
		Ed25519Public:  strings.TrimSpace(edPub) + "\n",
	}
	if ecdsaPriv, err := runOut(client, "cat /etc/ssh/ssh_host_ecdsa_key"); err == nil && strings.Contains(ecdsaPriv, "PRIVATE KEY") {
		out.ECDSAPrivate = strings.TrimSpace(ecdsaPriv) + "\n"
		if ecdsaPub, err := runOut(client, "cat /etc/ssh/ssh_host_ecdsa_key.pub"); err == nil {
			out.ECDSAPublic = strings.TrimSpace(ecdsaPub) + "\n"
		}
	}
	return out, nil
}

// InstallHostKeys writes host keys into the guest and restarts sshd.
// Used so recycled IPs keep a stable fingerprint for clients' known_hosts.
func InstallHostKeys(ctx context.Context, ip, password string, keys HostKeyMaterial) error {
	if strings.TrimSpace(keys.Ed25519Private) == "" || strings.TrimSpace(keys.Ed25519Public) == "" {
		return fmt.Errorf("sshavail: missing ed25519 host keys")
	}
	client, err := dialRoot(ctx, ip, password)
	if err != nil {
		return err
	}
	defer client.Close()

	script := buildInstallScript(keys)
	if err := run(client, script); err != nil {
		return fmt.Errorf("install host keys: %w", err)
	}
	return nil
}

func buildInstallScript(keys HostKeyMaterial) string {
	var b strings.Builder
	b.WriteString("set -euo pipefail; ")
	b.WriteString("umask 077; ")
	b.WriteString("cat > /etc/ssh/ssh_host_ed25519_key <<'EOF_CH_ED25519'\n")
	b.WriteString(ensureTrailingNewline(keys.Ed25519Private))
	b.WriteString("EOF_CH_ED25519\n")
	b.WriteString("cat > /etc/ssh/ssh_host_ed25519_key.pub <<'EOF_CH_ED25519_PUB'\n")
	b.WriteString(ensureTrailingNewline(keys.Ed25519Public))
	b.WriteString("EOF_CH_ED25519_PUB\n")
	if strings.Contains(keys.ECDSAPrivate, "PRIVATE KEY") && strings.TrimSpace(keys.ECDSAPublic) != "" {
		b.WriteString("cat > /etc/ssh/ssh_host_ecdsa_key <<'EOF_CH_ECDSA'\n")
		b.WriteString(ensureTrailingNewline(keys.ECDSAPrivate))
		b.WriteString("EOF_CH_ECDSA\n")
		b.WriteString("cat > /etc/ssh/ssh_host_ecdsa_key.pub <<'EOF_CH_ECDSA_PUB'\n")
		b.WriteString(ensureTrailingNewline(keys.ECDSAPublic))
		b.WriteString("EOF_CH_ECDSA_PUB\n")
		b.WriteString("chmod 600 /etc/ssh/ssh_host_ecdsa_key; chmod 644 /etc/ssh/ssh_host_ecdsa_key.pub; ")
	}
	b.WriteString("chmod 600 /etc/ssh/ssh_host_ed25519_key; chmod 644 /etc/ssh/ssh_host_ed25519_key.pub; ")
	b.WriteString("if command -v systemctl >/dev/null 2>&1; then systemctl restart ssh 2>/dev/null || systemctl restart sshd 2>/dev/null || true; ")
	b.WriteString("else service ssh restart 2>/dev/null || service sshd restart 2>/dev/null || true; fi")
	return b.String()
}

func ensureTrailingNewline(s string) string {
	s = strings.TrimRight(s, "\r\n") + "\n"
	return s
}

func dialRoot(ctx context.Context, ip, password string) (*ssh.Client, error) {
	ip = strings.TrimSpace(ip)
	password = strings.TrimSpace(password)
	if ip == "" || password == "" {
		return nil, fmt.Errorf("sshavail: missing ip or password")
	}
	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
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
	// Clear dial deadline so long remote scripts (software install) are not cut off.
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), nil
}

// TCPOpen reports whether ip:port accepts a TCP connection.
func TCPOpen(ctx context.Context, ip string, port int) bool {
	ip = strings.TrimSpace(ip)
	if ip == "" || port <= 0 {
		return false
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, fmt.Sprintf("%d", port)))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// PasswordAuthDisabled reports SSH failures where the guest only offers publickey
// (typical when VirtFusion injects SSH keys and cloud-init disables password login).
func PasswordAuthDisabled(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "publickey") ||
		strings.Contains(msg, "no supported methods") ||
		(strings.Contains(msg, "unable to authenticate") && strings.Contains(msg, "attempted methods"))
}

// InstallAuthorizedKeys writes OpenSSH public keys for root and optionally
// disables password authentication (when lockPassword is true).
func InstallAuthorizedKeys(ctx context.Context, ip, password string, publicKeys []string, lockPassword bool) error {
	cleaned := make([]string, 0, len(publicKeys))
	for _, k := range publicKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			cleaned = append(cleaned, k)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	client, err := dialRoot(ctx, ip, password)
	if err != nil {
		return err
	}
	defer client.Close()

	var b strings.Builder
	b.WriteString("set -euo pipefail\n")
	b.WriteString("mkdir -p /root/.ssh\n")
	b.WriteString("chmod 700 /root/.ssh\n")
	b.WriteString("cat > /root/.ssh/authorized_keys <<'EOF_CH_KEYS'\n")
	for _, k := range cleaned {
		b.WriteString(k)
		b.WriteByte('\n')
	}
	b.WriteString("EOF_CH_KEYS\n")
	b.WriteString("chmod 600 /root/.ssh/authorized_keys\n")
	b.WriteString("chown -R root:root /root/.ssh\n")
	if lockPassword {
		b.WriteString(`
mkdir -p /etc/ssh/sshd_config.d
printf 'PasswordAuthentication no\nKbdInteractiveAuthentication no\nChallengeResponseAuthentication no\n' > /etc/ssh/sshd_config.d/99-cloud-hustle-keys-only.conf
if [ -f /etc/ssh/sshd_config ]; then
  sed -i -E 's/^[#[:space:]]*PasswordAuthentication[[:space:]].*/PasswordAuthentication no/' /etc/ssh/sshd_config || true
  grep -qE '^PasswordAuthentication[[:space:]]+no' /etc/ssh/sshd_config || echo 'PasswordAuthentication no' >> /etc/ssh/sshd_config
fi
if command -v systemctl >/dev/null 2>&1; then
  systemctl restart ssh 2>/dev/null || systemctl restart sshd 2>/dev/null || true
else
  service ssh restart 2>/dev/null || service sshd restart 2>/dev/null || true
fi
`)
	}
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = strings.NewReader(b.String())
	var stderr bytes.Buffer
	session.Stderr = &stderr
	if err := session.Run("bash -s"); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("install authorized_keys: %w: %s", err, msg)
		}
		return fmt.Errorf("install authorized_keys: %w", err)
	}
	return nil
}

// RunScript dials root@ip and executes a remote shell script via bash -s (stdin).
func RunScript(ctx context.Context, ip, password, script string) error {
	client, err := dialRoot(ctx, ip, password)
	if err != nil {
		return err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = strings.NewReader(script)
	var stderr bytes.Buffer
	session.Stderr = &stderr
	if err := session.Run("bash -s"); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 2000 {
			msg = msg[len(msg)-2000:]
		}
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

// RunScriptOut is like RunScript but returns stdout.
func RunScriptOut(ctx context.Context, ip, password, script string) (string, error) {
	client, err := dialRoot(ctx, ip, password)
	if err != nil {
		return "", err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	session.Stdin = strings.NewReader(script)
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err := session.Run("bash -s"); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return stdout.String(), nil
}

func run(client *ssh.Client, cmd string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	var stderr bytes.Buffer
	session.Stderr = &stderr
	if err := session.Run(cmd); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

func runOut(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err := session.Run(cmd); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return stdout.String(), nil
}
