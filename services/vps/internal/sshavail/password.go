package sshavail

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// CheckUserPassword dials ip:22 and authenticates as user with password.
func CheckUserPassword(ctx context.Context, ip, user, password string, trustedPubKeys ...string) error {
	client, err := dialUserWithHostKeys(ctx, ip, user, password, trustedPubKeys)
	if err != nil {
		return err
	}
	defer client.Close()
	if isWindowsUser(user) {
		return run(client, "cmd /c exit 0")
	}
	return run(client, "true")
}

// ChangeUserPassword sets a new login password over SSH using the current password.
func ChangeUserPassword(ctx context.Context, ip, user, currentPassword, newPassword string, windows bool, trustedPubKeys ...string) error {
	currentPassword = strings.TrimSpace(currentPassword)
	newPassword = strings.TrimSpace(newPassword)
	if ip == "" || user == "" || currentPassword == "" || newPassword == "" {
		return fmt.Errorf("sshavail: missing ip, user, or password")
	}
	client, err := dialUserWithHostKeys(ctx, ip, user, currentPassword, trustedPubKeys)
	if err != nil {
		return err
	}
	defer client.Close()

	var cmd string
	if windows || isWindowsUser(user) {
		cmd = windowsChangePasswordCommand(user, newPassword)
	} else {
		cmd = fmt.Sprintf("echo %s:%s | chpasswd", shellSingleQuote(user), shellSingleQuote(newPassword))
	}
	if err := run(client, cmd); err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	if err := CheckUserPassword(ctx, ip, user, newPassword, trustedPubKeys...); err != nil {
		return fmt.Errorf("verify new password: %w", err)
	}
	return nil
}

// ApplyDesiredPassword tries to set desired on the guest using currentPassword for SSH auth.
// Returns desired when applied, or currentPassword when desired is empty or already active.
func ApplyDesiredPassword(ctx context.Context, ip, user, currentPassword, desired string, windows bool, trustedPubKeys ...string) (string, error) {
	desired = strings.TrimSpace(desired)
	currentPassword = strings.TrimSpace(currentPassword)
	if desired == "" {
		return currentPassword, nil
	}
	if ip == "" || user == "" {
		return currentPassword, fmt.Errorf("sshavail: missing ip or user")
	}
	if err := CheckUserPassword(ctx, ip, user, desired, trustedPubKeys...); err == nil {
		return desired, nil
	}
	if currentPassword == "" {
		return currentPassword, fmt.Errorf("sshavail: missing current password")
	}
	if err := ChangeUserPassword(ctx, ip, user, currentPassword, desired, windows, trustedPubKeys...); err != nil {
		return currentPassword, err
	}
	return desired, nil
}

func dialUser(ctx context.Context, ip, user, password string) (*ssh.Client, error) {
	return dialUserWithHostKeys(ctx, ip, user, password, nil)
}

func dialUserWithHostKeys(ctx context.Context, ip, user, password string, trustedPubKeys []string) (*ssh.Client, error) {
	ip = strings.TrimSpace(ip)
	user = strings.TrimSpace(user)
	password = strings.TrimSpace(password)
	if ip == "" || user == "" || password == "" {
		return nil, fmt.Errorf("sshavail: missing ip, user, or password")
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: HostKeyCallback(trustedPubKeys),
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

func isWindowsUser(user string) bool {
	switch strings.ToLower(strings.TrimSpace(user)) {
	case "administrator", "admin":
		return true
	default:
		return false
	}
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func windowsChangePasswordCommand(user, newPassword string) string {
	user = strings.TrimSpace(user)
	b64 := base64.StdEncoding.EncodeToString([]byte(newPassword))
	userQ := strings.ReplaceAll(user, "'", "''")
	return fmt.Sprintf(
		`powershell -NoProfile -NonInteractive -Command "$p = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('%s')); $sec = ConvertTo-SecureString $p -AsPlainText -Force; Set-LocalUser -Name '%s' -Password $sec"`,
		b64,
		userQ,
	)
}
