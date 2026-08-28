#!/usr/bin/env bash
# Fix QEMU guest agent on FI hypervisor via virsh (no guest root password / DB access).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT/infra/scripts/load-ops-env.sh"
load_ops_env "$ROOT"

: "${FI_SSH_PASS:?set FI_SSH_PASS in .env.probe}"
FI_IP="${FI_IP:-95.216.1.155}"
VF_ID="${1:-}"

ssh_hv "$FI_IP" "$FI_SSH_PASS" 'bash -s' <<'REMOTE'
set -euo pipefail
echo "=== FI hypervisor guest-agent audit ==="
supervisorctl status 2>/dev/null | grep -E 'vf-|hypervisor|queue' || true

fix_domain() {
  local dom="$1"
  [[ -z "$dom" ]] && return 0
  echo "--- domain=$dom ---"
  if virsh qemu-agent-command "$dom" '{"execute":"guest-ping"}' >/dev/null 2>&1; then
    echo "  guest-ping: OK"
    return 0
  fi
  echo "  guest-ping: FAIL — ensuring package via guest exec if possible"
  if ! virsh dumpxml "$dom" | grep -q 'org.qemu.guest_agent.0'; then
    echo "  WARNING: missing org.qemu.guest_agent.0 channel in libvirt XML"
  fi
  virsh qemu-agent-command "$dom" '{"execute":"guest-exec","arguments":{"path":"/bin/sh","arg":["-c","export DEBIAN_FRONTEND=noninteractive; apt-get update -qq && apt-get install -y -qq qemu-guest-agent && systemctl enable qemu-guest-agent && systemctl restart qemu-guest-agent"],"capture-output":true}}' 2>/dev/null || true
  sleep 3
  virsh qemu-agent-command "$dom" '{"execute":"guest-ping"}' 2>&1 || true
}

if [[ -n "${1:-}" ]]; then
  dom=$(virsh list --name | grep -E "^${1}-" | head -1 || true)
  if [[ -z "$dom" ]]; then
    echo "No running domain for VF id $1" >&2
    exit 1
  fi
  fix_domain "$dom"
else
  virsh list --name | while read -r dom; do
    fix_domain "$dom"
  done
fi
REMOTE

echo "VF_FIX_VM_GUEST_AGENT_DONE"
