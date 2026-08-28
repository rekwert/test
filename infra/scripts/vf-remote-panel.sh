#!/usr/bin/env bash
# Run a VF panel script on 66.248.206.14 from Back (requires NL_PASS or SSH key).
# Usage: NL_PASS='secret' bash infra/scripts/vf-remote-panel.sh vf-sync-windows-all-regions.sh
set -euo pipefail

PANEL_HOST="${VF_PANEL_HOST:-66.248.206.14}"
PANEL_USER="${VF_PANEL_USER:-root}"
SCRIPT="${1:-vf-sync-windows-all-regions.sh}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
REMOTE="/opt/testVPStrade/infra/scripts/$SCRIPT"

if [[ ! -f "$ROOT/infra/scripts/$SCRIPT" ]]; then
  echo "missing $ROOT/infra/scripts/$SCRIPT" >&2
  exit 1
fi

SSH_OPTS=(-o StrictHostKeyChecking=no -o ConnectTimeout=15)

run_ssh() {
  if [[ -n "${NL_PASS:-}" ]] && command -v sshpass >/dev/null; then
    sshpass -e ssh "${SSH_OPTS[@]}" "$PANEL_USER@$PANEL_HOST" "$@"
  else
    ssh "${SSH_OPTS[@]}" "$PANEL_USER@$PANEL_HOST" "$@"
  fi
}

echo "=== sync $SCRIPT to panel ==="
if [[ -n "${NL_PASS:-}" ]]; then
  export SSHPASS="$NL_PASS"
  sshpass -e scp "${SSH_OPTS[@]}" "$ROOT/infra/scripts/$SCRIPT" "$PANEL_USER@$PANEL_HOST:$REMOTE"
  if [[ -f "$ROOT/infra/windows/autounattend.xml" ]]; then
    sshpass -e scp "${SSH_OPTS[@]}" "$ROOT/infra/windows/autounattend.xml" "$PANEL_USER@$PANEL_HOST:/root/autounattend.xml"
  fi
else
  scp "${SSH_OPTS[@]}" "$ROOT/infra/scripts/$SCRIPT" "$PANEL_USER@$PANEL_HOST:$REMOTE"
  if [[ -f "$ROOT/infra/windows/autounattend.xml" ]]; then
    scp "${SSH_OPTS[@]}" "$ROOT/infra/windows/autounattend.xml" "$PANEL_USER@$PANEL_HOST:/root/autounattend.xml"
  fi
fi

echo "=== run on panel ==="
run_ssh "chmod +x $REMOTE && bash $REMOTE"
