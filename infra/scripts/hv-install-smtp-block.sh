#!/usr/bin/env bash
# Install default outbound SMTP block (TCP 25 + 2525) on a VirtFusion hypervisor.
# Exceptions are managed via ipset "smtp_allow" (platform admin open/close).
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
command -v ipset >/dev/null 2>&1 || { apt-get update -qq && apt-get install -y -qq ipset iptables; }

ipset create smtp_allow hash:ip family inet -exist

iptables -C FORWARD -m set --match-set smtp_allow src -p tcp -m multiport --dports 25,2525 -j ACCEPT 2>/dev/null || \
  iptables -I FORWARD 1 -m set --match-set smtp_allow src -p tcp -m multiport --dports 25,2525 -j ACCEPT

iptables -C FORWARD -p tcp -m multiport --dports 25,2525 -j DROP 2>/dev/null || \
  iptables -A FORWARD -p tcp -m multiport --dports 25,2525 -j DROP

# Persist if netfilter-persistent is available
if command -v netfilter-persistent >/dev/null 2>&1; then
  netfilter-persistent save || true
elif command -v iptables-save >/dev/null 2>&1; then
  mkdir -p /etc/iptables
  iptables-save > /etc/iptables/rules.v4 || true
  ipset save > /etc/iptables/ipset.smtp_allow || true
fi

echo "OK: outbound TCP 25,2525 DROP on FORWARD; allowlist ipset smtp_allow"
