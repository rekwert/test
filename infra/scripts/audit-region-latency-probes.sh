#!/usr/bin/env bash
# Security + functional audit for regional latency probe endpoints.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_PROBE="${ENV_PROBE:-$ROOT/infra/docker/.env.probe}"
FAIL=0
WARN=0

ok() { echo "OK   $*"; }
warn() { echo "WARN $*"; WARN=$((WARN + 1)); }
bad() { echo "FAIL $*"; FAIL=$((FAIL + 1)); }

if [[ -f "$ENV_PROBE" ]]; then
  # shellcheck disable=SC1090
  set -a && source "$ENV_PROBE" && set +a
fi

check_probe() {
  local name="$1" url="$2" want_ip="$3"
  local code wrong_code ip_code host resolved

  host="$(printf '%s' "$url" | sed -E 's#^https://([^/]+)/?.*#\1#')"
  resolved="$(getent ahosts "$host" 2>/dev/null | awk 'NR==1{print $1; exit}')"
  if [[ "$resolved" == "$want_ip" ]]; then
    ok "$name DNS $host -> $want_ip"
  else
    bad "$name DNS $host got '$resolved' want '$want_ip'"
  fi

  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$url" 2>/dev/null || true)
  code="${code:-000}"
  if [[ "$code" == "204" ]]; then
    ok "$name GET $url -> 204"
  else
    bad "$name GET $url -> $code (expected 204)"
  fi

  wrong_code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "${url%/}/admin" 2>/dev/null || true)
  wrong_code="${wrong_code:-000}"
  if [[ "$wrong_code" == "404" ]]; then
    ok "$name unknown path /admin -> 404"
  else
    bad "$name unknown path /admin -> $wrong_code (expected 404)"
  fi

  ip_code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 --resolve "${host}:443:${want_ip}" "https://${host}/" 2>/dev/null || true)
  ip_code="${ip_code:-000}"
  if [[ "$ip_code" == "204" ]]; then
    ok "$name HTTPS via IP+SNI -> 204"
  elif [[ "$name" == "NL" && "$ip_code" == "302" ]]; then
    ok "$name HTTPS via IP+SNI -> 302 (VF panel redirect, probe URL uses hostname)"
  else
    warn "$name HTTPS via IP+SNI -> $ip_code"
  fi

  if echo | openssl s_client -connect "${host}:443" -servername "$host" 2>/dev/null | openssl x509 -noout -subject 2>/dev/null | grep -q "$host"; then
    ok "$name TLS cert SAN matches $host"
  else
    bad "$name TLS cert mismatch for $host"
  fi
}

echo "=== Latency probe audit ==="

check_probe NL "https://panel.cloud-hustle.com/latency-probe" "66.248.206.14"
check_probe DE "https://probe-de.cloud-hustle.com/" "185.84.224.84"
check_probe FI "https://probe-fi.cloud-hustle.com/" "95.216.1.155"
check_probe GB "https://probe-gb.cloud-hustle.com/" "212.108.83.47"

echo "=== API regions probe_url ==="
api=$(curl -s --max-time 10 http://127.0.0.1:8080/api/v1/catalog/regions || true)
for pair in nl:panel.cloud-hustle.com/latency-probe de:probe-de fi:probe-fi gb:probe-gb; do
  code="${pair%%:*}"
  needle="${pair##*:}"
  if echo "$api" | grep -q "$needle"; then
    ok "API regions contains probe for $code ($needle)"
  else
    bad "API regions missing probe for $code ($needle)"
  fi
done

echo "=== Secrets file perms ==="
if [[ -f "$ENV_PROBE" ]]; then
  perms=$(stat -c '%a' "$ENV_PROBE" 2>/dev/null || stat -f '%OLp' "$ENV_PROBE" 2>/dev/null || echo ?)
  if [[ "$perms" == "600" ]]; then
    ok ".env.probe mode 600"
  else
    warn ".env.probe mode $perms (expected 600)"
  fi
else
  warn ".env.probe not found at $ENV_PROBE"
fi

echo "=== HV nginx exposure (DE sample) ==="
if [[ -n "${DE_SSH_PASS:-}" ]]; then
  SSHPASS="$DE_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 root@185.84.224.84 \
    'grep -E "server_name|listen|return|root " /etc/nginx/sites-available/latency-probe.conf 2>/dev/null | head -20' || warn "Could not read DE nginx conf"
  wrong=$(curl -s -o /dev/null -w '%{http_code}' --max-time 8 -k "https://185.84.224.84/" -H "Host: panel.cloud-hustle.com" 2>/dev/null || true)
  wrong="${wrong:-000}"
  if [[ "$wrong" == "444" || "$wrong" == "000" || "$wrong" == "421" || "$wrong" == "400" ]]; then
    ok "DE rejects wrong Host panel.cloud-hustle.com -> $wrong"
  else
    bad "DE accepts wrong Host panel.cloud-hustle.com -> $wrong (expected reject)"
  fi
fi

echo "---"
echo "FAIL=$FAIL WARN=$WARN"
[[ "$FAIL" -eq 0 ]]
