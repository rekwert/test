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

if ! command -v machinectl >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq systemd-container
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
  WRAP="/tmp/sunbeam-run-$$.sh"
  cat > "$WRAP" <<EOF
#!/bin/bash
set -euo pipefail
export PATH="/snap/bin:\${PATH}"
$CMD
EOF
  chown "${SUNBEAM_USER}:${SUNBEAM_USER}" "$WRAP"
  chmod 700 "$WRAP"
  /usr/bin/machinectl shell "${SUNBEAM_USER}@" "$WRAP"
  rm -f "$WRAP"
else
  # Fallback: localhost SSH with key (no sunbeam password).
  SUNBEAM_HOME="$(getent passwd "$SUNBEAM_USER" | cut -d: -f6)"
  install -d -m 700 -o "$SUNBEAM_USER" -g "$SUNBEAM_USER" "$SUNBEAM_HOME/.ssh"
  KEY="/root/.ssh/id_ed25519_sunbeam_local"
  if [[ ! -f "$KEY" ]]; then
    ssh-keygen -t ed25519 -N "" -f "$KEY" >/dev/null
  fi
  AUTH="$SUNBEAM_HOME/.ssh/authorized_keys"
  grep -qF "$(cat "${KEY}.pub")" "$AUTH" 2>/dev/null || cat "${KEY}.pub" >> "$AUTH"
  chown "$SUNBEAM_USER:$SUNBEAM_USER" "$AUTH"
  chmod 600 "$AUTH"
  ssh -i "$KEY" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    "${SUNBEAM_USER}@127.0.0.1" "export PATH=/snap/bin:\$PATH; $CMD"
fi
