package sshavail

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ApplyAllIPv4OnGuest configures every assigned IPv4 on the primary interface.
// reachIP must be an address that currently accepts root SSH.
func ApplyAllIPv4OnGuest(ctx context.Context, reachIP, password, primaryIP, gateway string, prefix int, allIPs []string) error {
	reachIP = strings.TrimSpace(reachIP)
	password = strings.TrimSpace(password)
	primaryIP = strings.TrimSpace(primaryIP)
	gateway = strings.TrimSpace(gateway)
	if reachIP == "" || password == "" || len(allIPs) == 0 {
		return fmt.Errorf("sshavail: reachIP, password and allIPs required")
	}
	if prefix <= 0 || prefix > 32 {
		prefix = 24
	}
	if gateway == "" {
		parts := strings.Split(primaryIP, ".")
		if len(parts) != 4 {
			parts = strings.Split(allIPs[0], ".")
		}
		if len(parts) == 4 {
			gateway = parts[0] + "." + parts[1] + "." + parts[2] + ".1"
		}
	}

	ips := normalizeIPList(allIPs, primaryIP)
	script := buildMultiIPScript(ips, primaryIP, gateway, prefix)
	return RunScript(ctx, reachIP, password, script)
}

// DialAnyRoot connects via the first IP in candidates that accepts root password auth.
func DialAnyRoot(ctx context.Context, candidates []string, password string) (reachable string, err error) {
	password = strings.TrimSpace(password)
	for _, ip := range candidates {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if err := CheckRootPassword(ctx, ip, password); err == nil {
			return ip, nil
		}
	}
	return "", fmt.Errorf("sshavail: no reachable ip for root ssh")
}

func normalizeIPList(allIPs []string, primaryIP string) []string {
	seen := make(map[string]struct{}, len(allIPs))
	out := make([]string, 0, len(allIPs))
	add := func(ip string) {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			return
		}
		if _, ok := seen[ip]; ok {
			return
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}
	add(primaryIP)
	for _, ip := range allIPs {
		add(ip)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i] == primaryIP {
			return true
		}
		if out[j] == primaryIP {
			return false
		}
		return out[i] < out[j]
	})
	return out
}

func buildMultiIPScript(allIPs []string, primaryIP, gateway string, prefix int) string {
	pfx := strconv.Itoa(prefix)
	var addrLines strings.Builder
	for _, ip := range allIPs {
		fmt.Fprintf(&addrLines, "        - %s/%s\n", ip, pfx)
	}
	var liveAdd strings.Builder
	for _, ip := range allIPs {
		fmt.Fprintf(&liveAdd, "ip addr replace %s/%s dev \"$IFACE\" 2>/dev/null || ip addr add %s/%s dev \"$IFACE\" 2>/dev/null || true\n", ip, pfx, ip, pfx)
	}

	var b strings.Builder
	b.WriteString("set -euo pipefail\n")
	b.WriteString("IFACE=$(ip -o link show | awk -F': ' '$2!~/lo/{print $2; exit}')\n")
	b.WriteString("test -n \"$IFACE\"\n")
	b.WriteString(fmt.Sprintf("GW=%q\n", gateway))
	b.WriteString(fmt.Sprintf("PFX=%s\n", pfx))
	b.WriteString(liveAdd.String())
	b.WriteString(`ip route replace default via "$GW" dev "$IFACE"
mkdir -p /etc/network/interfaces.d /etc/cloud/cloud.cfg.d
echo 'network: {config: disabled}' > /etc/cloud/cloud.cfg.d/99-disable-network-config.cfg
`)
	if primaryIP != "" {
		b.WriteString(fmt.Sprintf(`
cat > /etc/network/interfaces.d/50-static <<EOF
auto $IFACE
iface $IFACE inet static
    address %s/%s
    gateway %s
    dns-nameservers 1.1.1.1 8.8.8.8
EOF
`, primaryIP, pfx, gateway))
		for _, ip := range allIPs {
			if ip == primaryIP {
				continue
			}
			fmt.Fprintf(&b, `
cat >> /etc/network/interfaces.d/50-static <<EOF
    up ip addr add %s/%s dev $IFACE || true
EOF
`, ip, pfx)
		}
	}
	b.WriteString(fmt.Sprintf(`
if ls /etc/netplan/*.yaml >/dev/null 2>&1; then
  cat > /etc/netplan/50-static.yaml <<EOF
network:
  version: 2
  ethernets:
    $IFACE:
      addresses:
%s      routes:
        - to: default
          via: %s
      nameservers:
        addresses: [1.1.1.1, 8.8.8.8]
EOF
  netplan apply 2>/dev/null || true
fi
ip -4 -br addr show "$IFACE"
`, addrLines.String(), gateway))
	return b.String()
}
