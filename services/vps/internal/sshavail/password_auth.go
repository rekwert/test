package sshavail

import "context"

const enableRootPasswordAuthScript = `
set -euo pipefail
rm -f /etc/ssh/sshd_config.d/99-cloud-hustle-keys-only.conf
if [ -f /etc/ssh/sshd_config ]; then
  sed -i -E 's/^[#[:space:]]*PasswordAuthentication[[:space:]].*/PasswordAuthentication yes/' /etc/ssh/sshd_config || true
  grep -qE '^PasswordAuthentication[[:space:]]+yes' /etc/ssh/sshd_config || echo 'PasswordAuthentication yes' >> /etc/ssh/sshd_config
  sed -i -E 's/^[#[:space:]]*KbdInteractiveAuthentication[[:space:]].*/KbdInteractiveAuthentication yes/' /etc/ssh/sshd_config || true
fi
if command -v systemctl >/dev/null 2>&1; then
  systemctl restart ssh 2>/dev/null || systemctl restart sshd 2>/dev/null || true
else
  service ssh restart 2>/dev/null || service sshd restart 2>/dev/null || true
fi
`

// EnableRootPasswordAuth re-enables root password login (used before automation installs ops keys).
func EnableRootPasswordAuth(ctx context.Context, ip string, auth GuestSSHAuth) error {
	return RunScriptAuth(ctx, ip, auth, enableRootPasswordAuthScript)
}
