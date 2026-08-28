package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const ipProbeTimeout = 4 * time.Second

type IPCheckOptions struct {
	SSHPort int `json:"ssh_port"`
}

type ipCheckAttempt struct {
	N          int    `json:"n"`
	OK         bool   `json:"ok"`
	Protocol   string `json:"protocol"`
	Port       int    `json:"port,omitempty"`
	Detail     string `json:"detail"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

func normalizeSSHPort(port int) int {
	if port < 1 || port > 65535 {
		return 0
	}
	return port
}

func RunIPBlockCheck(ctx context.Context, ip string, opts IPCheckOptions) map[string]any {
	opts.SSHPort = normalizeSSHPort(opts.SSHPort)
	if url := strings.TrimSpace(os.Getenv("RU_IP_PROBE_URL")); url != "" {
		if res, err := probeViaHTTP(ctx, url, ip, opts); err == nil {
			return res
		}
	}
	if sshTarget := strings.TrimSpace(os.Getenv("RU_IP_PROBE_SSH")); sshTarget != "" {
		if res, err := probeViaSSH(ctx, sshTarget, ip, opts); err == nil {
			return res
		}
	}
	res := probeIPLocal(ip, opts)
	res["source"] = "infra"
	return res
}

func probeViaHTTP(ctx context.Context, url, ip string, opts IPCheckOptions) (map[string]any, error) {
	payload := map[string]any{"ip": ip}
	if opts.SSHPort > 0 {
		payload["ssh_port"] = opts.SSHPort
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(os.Getenv("RU_IP_PROBE_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("probe http %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	out["source"] = "ru"
	return out, nil
}

func probeViaSSH(ctx context.Context, target, ip string, opts IPCheckOptions) (map[string]any, error) {
	scriptPath := strings.TrimSpace(os.Getenv("RU_IP_PROBE_SCRIPT"))
	if scriptPath == "" {
		scriptPath = "/opt/testVPStrade/infra/scripts/ru-ip-probe.py"
	}
	args := []string{"python3", scriptPath, ip}
	if opts.SSHPort > 0 {
		args = append(args, strconv.Itoa(opts.SSHPort))
	}
	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=8",
		target,
	)
	cmd.Args = append(cmd.Args, args...)
	raw, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(raw), &out); err != nil {
		return nil, err
	}
	out["source"] = "ru"
	return out, nil
}

func probeIPLocal(ip string, opts IPCheckOptions) map[string]any {
	start := time.Now()
	attempts := make([]ipCheckAttempt, 0, 4)
	n := 0

	ssh22OK, ssh22MS, ssh22Detail := probeSSHHandshakeOnPort(ip, 22)
	n++
	attempts = append(attempts, ipCheckAttempt{
		N: n, OK: ssh22OK, Protocol: "SSH", Port: 22, Detail: ssh22Detail, DurationMS: ssh22MS,
	})

	customOK := false
	customMS := int64(0)
	customDetail := ""
	customPort := opts.SSHPort
	if customPort > 0 && customPort != 22 {
		customOK, customMS, customDetail = probeSSHHandshakeOnPort(ip, customPort)
		n++
		attempts = append(attempts, ipCheckAttempt{
			N: n, OK: customOK, Protocol: "SSH", Port: customPort, Detail: customDetail, DurationMS: customMS,
		})
	}

	sshOK := ssh22OK || customOK
	var rdpOK bool
	var rdpMS int64
	var rdpDetail string
	if !sshOK {
		rdpOK, rdpMS, rdpDetail = probeRDPHandshake(ip)
		n++
		attempts = append(attempts, ipCheckAttempt{
			N: n, OK: rdpOK, Protocol: "RDP", Port: 3389, Detail: rdpDetail, DurationMS: rdpMS,
		})
	}

	ok := sshOK || rdpOK
	status := "fail"
	reason := "all_handshakes_failed"
	reasonText := "Handshake не пройден на 22/3389"
	if customPort > 0 && customPort != 22 && !ok {
		reasonText += fmt.Sprintf(" и на SSH %d", customPort)
	}
	reasonText += " — возможна блокировка РКN или закрыты порты"
	protocol := ""

	switch {
	case ssh22OK:
		status = "pass"
		reason = "standard_ssh_passed"
		reasonText = "SSH на порту 22 доступен из РФ"
		protocol = "SSH · 22"
	case customOK:
		status = "pass"
		reason = "custom_ssh_passed"
		reasonText = fmt.Sprintf(
			"SSH доступен из РФ на порту %d. Порт 22 закрыт — это настройка firewall клиента, а не блокировка РКN",
			customPort,
		)
		protocol = fmt.Sprintf("SSH · %d", customPort)
	case rdpOK:
		status = "pass"
		reason = "rdp_handshake_passed"
		reasonText = "RDP handshake пройден из РФ"
		protocol = "RDP · 3389"
	case !ssh22OK && customPort == 0:
		reasonText += ". Если SSH на нестандартном порту — укажите его в проверке"
	}

	success := 0
	for _, a := range attempts {
		if a.OK {
			success++
		}
	}

	attemptOut := make([]map[string]any, 0, len(attempts))
	for _, a := range attempts {
		row := map[string]any{
			"n": a.N, "ok": a.OK, "protocol": a.Protocol,
			"detail": a.Detail, "duration_ms": a.DurationMS,
		}
		if a.Port > 0 {
			row["port"] = a.Port
			if a.Protocol == "SSH" {
				row["protocol"] = fmt.Sprintf("SSH · %d", a.Port)
			}
		}
		attemptOut = append(attemptOut, row)
	}

	out := map[string]any{
		"ip":               ip,
		"ok":               ok,
		"status":           status,
		"protocol":         protocol,
		"duration_ms":      time.Since(start).Milliseconds(),
		"reason":           reason,
		"reason_text":      reasonText,
		"attempts_total":   len(attempts),
		"attempts_success": success,
		"attempts":         attemptOut,
		"ssh_22":           portProbeResult(ssh22OK, ssh22MS, ssh22Detail, false),
		"rdp_3389":         portProbeResult(rdpOK, rdpMS, rdpDetail, sshOK),
	}
	if customPort > 0 && customPort != 22 {
		out["ssh_custom"] = portProbeResult(customOK, customMS, customDetail, false)
		out["ssh_port_checked"] = customPort
	}
	return out
}

func portProbeResult(ok bool, latencyMS int64, detail string, skipped bool) map[string]any {
	out := map[string]any{
		"reachable":  ok,
		"handshake":  ok,
		"skipped":    skipped,
		"latency_ms": latencyMS,
	}
	if !ok && detail != "" {
		out["error"] = detail
	}
	return out
}

func probeSSHHandshake(ip string) (bool, int64, string) {
	return probeSSHHandshakeOnPort(ip, 22)
}

func probeSSHHandshakeOnPort(ip string, port int) (bool, int64, string) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), ipProbeTimeout)
	if err != nil {
		return false, time.Since(start).Milliseconds(), "TCP недоступен"
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return false, time.Since(start).Milliseconds(), "SSH banner не получен"
	}
	banner := strings.TrimSpace(string(buf[:n]))
	if strings.HasPrefix(banner, "SSH-") {
		return true, time.Since(start).Milliseconds(), "SSH version exchange успешен"
	}
	return false, time.Since(start).Milliseconds(), "Некорректный SSH banner"
}

func probeRDPHandshake(ip string) (bool, int64, string) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "3389"), ipProbeTimeout)
	if err != nil {
		return false, time.Since(start).Milliseconds(), "TCP недоступен"
	}
	defer conn.Close()

	req := []byte{
		0x03, 0x00, 0x00, 0x13, 0x0e, 0xe0, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x08, 0x00, 0x00,
		0x00, 0x00, 0x00,
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write(req); err != nil {
		return false, time.Since(start).Milliseconds(), "RDP запрос не отправлен"
	}
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil || n < 7 {
		return false, time.Since(start).Milliseconds(), "RDP ответ не получен"
	}
	if buf[0] == 0x03 && buf[5] == 0xd0 {
		return true, time.Since(start).Milliseconds(), "RDP handshake успешен"
	}
	return false, time.Since(start).Milliseconds(), "RDP handshake отклонён"
}
