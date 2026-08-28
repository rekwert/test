#!/usr/bin/env bash
# Run a command as SUNBEAM_USER with a real systemd user session (required for snap/sunbeam).
set -euo pipefail

SUNBEAM_USER="${SUNBEAM_USER:-sunbeam}"
CMD="${1:?usage: sunbeam-run-as-user.sh '<shell command>'}"

if ! id "$SUNBEAM_USER" &>/dev/null; then
  echo "User $SUNBEAM_USER does not exist"
  exit 1
fi

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root"
  exit 1
fi

# Passwordless sudo for prepare-node-script (it configures this itself too).
SUDOERS="/etc/sudoers.d/90-${SUNBEAM_USER}-sunbeam"
if [[ ! -f "$SUDOERS" ]]; then
  echo "${SUNBEAM_USER} ALL=(ALL) NOPASSWD:ALL" > "$SUDOERS"
  chmod 440 "$SUDOERS"
fi

loginctl enable-linger "$SUNBEAM_USER" 2>/dev/null || true

export PATH="/snap/bin:${PATH:-}"

if command -v machinectl >/dev/null 2>&1; then
  machinectl shell "${SUNBEAM_USER}@" /bin/bash -lc "$CMD"
else
  # Fallback: SSH loopback creates a proper login session (set password: passwd sunbeam).
  exec ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    "${SUNBEAM_USER}@127.0.0.1" "export PATH=/snap/bin:\$PATH; $CMD"
fi
