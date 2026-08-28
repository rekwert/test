#!/usr/bin/env bash
# Deploy regional HTTPS latency probes for browser RTT measurement.
#
# Prerequisites:
#   1) DNS A records in Selectel for cloud-hustle.com (TTL 300):
#        probe-de.cloud-hustle.com -> 185.84.224.84
#        probe-fi.cloud-hustle.com -> 95.216.1.155
#        probe-gb.cloud-hustle.com -> 212.108.83.47
#   2) SSH passwords exported (or in /opt/testVPStrade/infra/docker/.env.probe):
#        NL_SSH_PASS DE_SSH_PASS FI_SSH_PASS GB_SSH_PASS
#
# NL uses https://panel.cloud-hustle.com/latency-probe (no extra DNS).
#
# Usage (from back host):
#   bash infra/scripts/deploy-region-latency-probes.sh
#   bash infra/scripts/deploy-region-latency-probes.sh --skip-dns-check
#   bash infra/scripts/deploy-region-latency-probes.sh --dns-only
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_PROBE="${ENV_PROBE:-$ROOT/infra/docker/.env.probe}"
CERTBOT_EMAIL="${CERTBOT_EMAIL:-admin@cloud-hustle.com}"

NL_IP="${NL_IP:-66.248.206.14}"
DE_IP="${DE_IP:-185.84.224.84}"
FI_IP="${FI_IP:-95.216.1.155}"
GB_IP="${GB_IP:-212.108.83.47}"

if [[ -f "$ENV_PROBE" ]]; then
  # shellcheck disable=SC1090
  set -a && source "$ENV_PROBE" && set +a
fi

: "${NL_SSH_PASS:?set NL_SSH_PASS (NL panel root password)}"
: "${DE_SSH_PASS:?set DE_SSH_PASS}"
: "${FI_SSH_PASS:?set FI_SSH_PASS}"
: "${GB_SSH_PASS:?set GB_SSH_PASS}"

ssh_nl() {
  SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no -o ConnectTimeout=20 "root@${NL_IP}" "$@"
}

ssh_hv() {
  local ip="$1" pass="$2"
  shift 2
  SSHPASS="$pass" sshpass -e ssh -o StrictHostKeyChecking=no -o ConnectTimeout=20 -o UserKnownHostsFile=/dev/null "root@${ip}" "$@"
}

scp_hv() {
  local ip="$1" pass="$2" src="$3" dst="$4"
  SSHPASS="$pass" sshpass -e scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "$src" "root@${ip}:${dst}"
}

dns_expect() {
  local fqdn="$1" want_ip="$2"
  local got
  got="$(getent ahostsv4 "$fqdn" 2>/dev/null | awk '{print $1; exit}' || true)"
  [[ "$got" == "$want_ip" ]]
}

print_dns_instructions() {
  cat <<EOF

Add these DNS A records in Selectel (cloud-hustle.com zone):

  probe-de.cloud-hustle.com  A  ${DE_IP}
  probe-fi.cloud-hustle.com  A  ${FI_IP}
  probe-gb.cloud-hustle.com  A  ${GB_IP}

NL probe uses existing panel URL (no new DNS):
  https://panel.cloud-hustle.com/latency-probe

Wait 2–10 minutes for propagation, then re-run this script.

EOF
}

wait_for_dns() {
  local tries="${1:-30}"
  local fqdn ip
  for fqdn_ip in \
    "probe-de.cloud-hustle.com|${DE_IP}" \
    "probe-fi.cloud-hustle.com|${FI_IP}" \
    "probe-gb.cloud-hustle.com|${GB_IP}"; do
    fqdn="${fqdn_ip%%|*}"
    ip="${fqdn_ip##*|}"
    local ok=0
    for ((i=1; i<=tries; i++)); do
      if dns_expect "$fqdn" "$ip"; then
        echo "DNS OK  ${fqdn} -> ${ip}"
        ok=1
        break
      fi
      echo "DNS wait ${fqdn} -> ${ip} (${i}/${tries})"
      sleep 10
    done
    if [[ "$ok" != "1" ]]; then
      echo "DNS missing: ${fqdn} should point to ${ip}" >&2
      return 1
    fi
  done
}

if [[ "${1:-}" == "--dns-only" ]]; then
  print_dns_instructions
  exit 0
fi

if [[ "${1:-}" != "--skip-dns-check" ]]; then
  print_dns_instructions
  if ! wait_for_dns 30; then
    echo "Aborting: fix DNS in Selectel and re-run." >&2
    exit 2
  fi
fi

echo "=== NL panel: /latency-probe ==="
scp_nl() {
  SSHPASS="$NL_SSH_PASS" sshpass -e scp -o StrictHostKeyChecking=no "$ROOT/infra/scripts/region-latency-probe-nl-vf.sh" "root@${NL_IP}:/tmp/region-latency-probe-nl-vf.sh"
}
scp_nl
ssh_nl "sed -i 's/\r$//' /tmp/region-latency-probe-nl-vf.sh && bash /tmp/region-latency-probe-nl-vf.sh"

deploy_hv() {
  local code="$1" ip="$2" pass="$3" fqdn="$4"
  echo "=== ${code} hypervisor: ${fqdn} on ${ip} ==="
  scp_hv "$ip" "$pass" "$ROOT/infra/scripts/region-latency-probe-hypervisor.sh" /tmp/region-latency-probe-hypervisor.sh
  ssh_hv "$ip" "$pass" "sed -i 's/\r$//' /tmp/region-latency-probe-hypervisor.sh && FQDN=${fqdn} CERTBOT_EMAIL=${CERTBOT_EMAIL} bash /tmp/region-latency-probe-hypervisor.sh"
  local ext_code
  ext_code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 "https://${fqdn}/" || echo 000)
  echo "${fqdn} external_https=${ext_code}"
  if [[ "$ext_code" != "204" ]]; then
    echo "WARN external probe check failed for ${fqdn}" >&2
  fi
}

deploy_hv de "$DE_IP" "$DE_SSH_PASS" probe-de.cloud-hustle.com
deploy_hv fi "$FI_IP" "$FI_SSH_PASS" probe-fi.cloud-hustle.com
deploy_hv gb "$GB_IP" "$GB_SSH_PASS" probe-gb.cloud-hustle.com

PROBE_URLS="nl=https://panel.cloud-hustle.com/latency-probe,de=https://probe-de.cloud-hustle.com/,fi=https://probe-fi.cloud-hustle.com/,gb=https://probe-gb.cloud-hustle.com/"

echo "=== Update back .env REGION_PROBE_URLS ==="
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
if [[ -f "$ENV_FILE" ]]; then
  if grep -q '^REGION_PROBE_URLS=' "$ENV_FILE"; then
    sed -i "s|^REGION_PROBE_URLS=.*|REGION_PROBE_URLS=${PROBE_URLS}|" "$ENV_FILE"
  else
    echo "REGION_PROBE_URLS=${PROBE_URLS}" >>"$ENV_FILE"
  fi
  echo "Updated $ENV_FILE"
  cd "$ROOT/infra/docker"
  docker compose -f docker-compose.back.yml up -d --force-recreate vps gateway >/dev/null
  echo "Restarted vps + gateway containers"
else
  echo "Set on back manually:"
  echo "REGION_PROBE_URLS=${PROBE_URLS}"
fi

echo "=== Verify public API ==="
sleep 3
curl -s "http://127.0.0.1:8080/api/v1/catalog/regions" | head -c 800
echo ""
echo "DEPLOY_REGION_LATENCY_PROBES_DONE"
