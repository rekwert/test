#!/usr/bin/env bash
# Install SMTP outbound block (TCP 25/2525) on every VirtFusion hypervisor host.
# Safe: only adds ipset smtp_allow + FORWARD ACCEPT/DROP rules; does not flush iptables.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT="$ROOT/infra/scripts/hv-install-smtp-block.sh"
ENV_FILE="${ENV_FILE:-$ROOT/infra/docker/.env}"
PROBE_ENV="${PROBE_ENV:-$ROOT/infra/docker/.env.probe}"

if [[ ! -f "$SCRIPT" ]]; then
  echo "Missing $SCRIPT" >&2
  exit 1
fi

# shellcheck disable=SC1090
source "$ENV_FILE"
[[ -f "$PROBE_ENV" ]] && source "$PROBE_ENV"

# Sync per-region HV passwords into main .env for vps container (idempotent append).
ensure_env_kv() {
  local key="$1" val="$2"
  [[ -z "$val" ]] && return 0
  if grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
    return 0
  fi
  echo "${key}=${val}" >> "$ENV_FILE"
}
ensure_env_kv DE_SSH_PASS "${DE_SSH_PASS:-}"
ensure_env_kv DE_MID_SSH_PASS "${DE_MID_SSH_PASS:-}"
ensure_env_kv FI_SSH_PASS "${FI_SSH_PASS:-}"
ensure_env_kv GB_SSH_PASS "${GB_SSH_PASS:-}"
ensure_env_kv NL_SSH_PASS "${NL_SSH_PASS:-}"

SSH_USER="${VIRTFUSION_CTRL_SSH_USER:-root}"
SSH_PORT="${VIRTFUSION_CTRL_SSH_PORT:-22}"
SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=20 -p "$SSH_PORT")

declare -A HV_SEEN=()
HV_HOSTS=()

add_hv() {
  local ip="${1// /}"
  [[ -z "$ip" ]] && return 0
  [[ -n "${HV_SEEN[$ip]:-}" ]] && return 0
  HV_SEEN[$ip]=1
  HV_HOSTS+=("$ip")
}

echo "=== Collecting hypervisor IPs from portal DB ==="
while IFS= read -r ip; do
  add_hv "$ip"
done < <(psql "$POSTGRES_DSN" -At -c "
  SELECT DISTINCT TRIM(vf_ip)
  FROM vps.nodes
  WHERE vf_ip IS NOT NULL AND TRIM(vf_ip) <> ''
  ORDER BY 1;
")

echo "=== Collecting hypervisor IPs from VirtFusion MySQL (if reachable) ==="
VF_CTRL="${VIRTFUSION_CTRL_SSH_HOST:-66.248.206.14}"
if command -v sshpass >/dev/null 2>&1 && [[ -n "${NL_SSH_PASS:-}" ]]; then
  while IFS= read -r ip; do
    add_hv "$ip"
  done < <(SSHPASS="$NL_SSH_PASS" sshpass -e ssh "${SSH_OPTS[@]}" root@"$VF_CTRL" bash -s <<'VF' 2>/dev/null || true
set -a; source /opt/virtfusion/app/control/.env 2>/dev/null || exit 0; set +a
mysql -N -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" \
  -e "SELECT DISTINCT ip FROM hypervisors WHERE ip IS NOT NULL AND ip <> '' ORDER BY ip;" 2>/dev/null || true
VF
)
fi

if [[ ${#HV_HOSTS[@]} -eq 0 ]]; then
  echo "No hypervisor IPs found." >&2
  exit 1
fi

echo "=== Hypervisors to configure (${#HV_HOSTS[@]}) ==="
printf '  %s\n' "${HV_HOSTS[@]}"

ssh_with_auth() {
  local host="$1"
  shift
  local pass=""
  case "$host" in
    212.102.227.6) pass="${DE_SSH_PASS:-}" ;;
    212.102.227.7) pass="${DE_MID_SSH_PASS:-${DE_SSH_PASS:-}}" ;;
    95.216.1.155) pass="${FI_SSH_PASS:-}" ;;
    212.108.83.47) pass="${GB_SSH_PASS:-}" ;;
    66.248.206.14) pass="${NL_SSH_PASS:-${VIRTFUSION_CTRL_SSH_PASSWORD:-}}" ;;
    *) pass="${VIRTFUSION_CTRL_SSH_PASSWORD:-${NL_SSH_PASS:-}}" ;;
  esac
  if [[ -n "${VIRTFUSION_CTRL_SSH_PRIVATE_KEY:-}" ]]; then
    local keyfile
    keyfile="$(mktemp)"
    trap 'rm -f "$keyfile"' RETURN
    printf '%s\n' "$VIRTFUSION_CTRL_SSH_PRIVATE_KEY" > "$keyfile"
    chmod 600 "$keyfile"
    ssh "${SSH_OPTS[@]}" -i "$keyfile" "$SSH_USER@$host" "$@"
  elif [[ -n "$pass" ]]; then
    SSHPASS="$pass" sshpass -e ssh "${SSH_OPTS[@]}" "$SSH_USER@$host" "$@"
  elif [[ -n "${VIRTFUSION_CTRL_SSH_PASSWORD:-}" ]]; then
    SSHPASS="$VIRTFUSION_CTRL_SSH_PASSWORD" sshpass -e ssh "${SSH_OPTS[@]}" "$SSH_USER@$host" "$@"
  else
    ssh "${SSH_OPTS[@]}" "$SSH_USER@$host" "$@"
  fi
}

VERIFY_CMD='
echo "--- ipset smtp_allow ---"
ipset list smtp_allow 2>/dev/null | head -20 || echo "(empty or missing)"
echo "--- FORWARD rules (25/2525) ---"
iptables -L FORWARD -n -v --line-numbers 2>/dev/null | grep -E "smtp_allow|multiport dports 25,2525|dpt:25|dpt:2525" || true
'

FAILED=0
for hv in "${HV_HOSTS[@]}"; do
  echo ""
  echo "========== HV $hv =========="
  if ! ssh_with_auth "$hv" "bash -s" < "$SCRIPT"; then
    echo "FAIL: install on $hv" >&2
    FAILED=$((FAILED + 1))
    continue
  fi
  echo "--- verify $hv ---"
  ssh_with_auth "$hv" "bash -lc $(printf '%q' "$VERIFY_CMD")" || true
done

echo ""
echo "=== SSH reachability (same creds as vps smtpblock) ==="
for hv in "${HV_HOSTS[@]}"; do
  if ssh_with_auth "$hv" "echo OK-ssh"; then
    echo "  back -> $hv: OK"
  else
    echo "  back -> $hv: FAIL" >&2
    FAILED=$((FAILED + 1))
  fi
done

echo ""
echo "=== vps container HV password env ==="
VPS_CID="$(docker ps -q -f name=docker-vps-1 2>/dev/null | head -1)"
if [[ -n "$VPS_CID" ]]; then
  for k in NL_SSH_PASS DE_SSH_PASS DE_MID_SSH_PASS FI_SSH_PASS GB_SSH_PASS VIRTFUSION_CTRL_SSH_PASSWORD; do
    if docker exec "$VPS_CID" sh -c "test -n \"\${$k:-}\"" 2>/dev/null; then
      echo "  $k: set"
    else
      echo "  $k: missing" >&2
    fi
  done
else
  echo "WARN: vps container not found"
fi

echo ""
if [[ "$FAILED" -gt 0 ]]; then
  echo "Done with $FAILED failure(s)." >&2
  exit 1
fi
echo "All hypervisors configured and verified."
