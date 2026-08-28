#!/usr/bin/env bash
# Preflight from NL back VPS (or any host with OPENSTACK_* env). Exit 0 = ready for worker.
set -euo pipefail

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "OK: $*"; }

[[ -n "${OPENSTACK_AUTH_URL:-}" ]] || fail "OPENSTACK_AUTH_URL not set"
[[ "${OPENSTACK_USE_MOCK:-false}" == "true" ]] && fail "OPENSTACK_USE_MOCK=true — set false for real cloud"

echo "=== OpenStack preflight ==="
echo "AUTH_URL=$OPENSTACK_AUTH_URL"

if command -v curl >/dev/null 2>&1; then
  curl -fsS -k -o /dev/null "${OPENSTACK_AUTH_URL%/}/" && ok "Keystone URL reachable" || fail "Keystone unreachable at $OPENSTACK_AUTH_URL"
fi

if ! command -v openstack >/dev/null 2>&1; then
  echo "WARN: openstack CLI not installed — skip API checks (install python3-openstackclient on back VPS optional)"
  exit 0
fi

openstack token issue -f value -c id >/dev/null && ok "Authentication"
openstack flavor list -f value -c Name | grep -qx "m1.small" && ok "flavor m1.small" || echo "WARN: flavor m1.small missing — create before provision"
openstack image list -f value -c Name | grep -Eiq "ubuntu-22.04|ubuntu" && ok "ubuntu image" || echo "WARN: ubuntu image missing"
[[ -n "${OPENSTACK_NETWORK_ID:-}" ]] && openstack network show "$OPENSTACK_NETWORK_ID" >/dev/null && ok "tenant network $OPENSTACK_NETWORK_ID" || echo "WARN: OPENSTACK_NETWORK_ID unset or invalid"
[[ -n "${OPENSTACK_FLOATING_NETWORK_ID:-}" ]] && openstack network show "$OPENSTACK_FLOATING_NETWORK_ID" >/dev/null && ok "floating network $OPENSTACK_FLOATING_NETWORK_ID" || echo "WARN: OPENSTACK_FLOATING_NETWORK_ID unset or invalid"
openstack hypervisor list -f value -c "Hypervisor Hostname" | head -3 && ok "hypervisors registered"

[[ -n "${OPENSTACK_PLAN_MAP:-}" ]] || echo "WARN: OPENSTACK_PLAN_MAP empty in .env"
[[ -n "${OPENSTACK_OS_MAP:-}" ]] || echo "WARN: OPENSTACK_OS_MAP empty in .env"

echo "=== Preflight done ==="
