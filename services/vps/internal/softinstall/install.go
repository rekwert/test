package softinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/sshavail"
)

// Apply installs a software profile on a freshly provisioned Linux guest.
// clean / empty / windows → no-op. Failures are returned but callers may treat them as non-fatal.
func Apply(ctx context.Context, ip, password, osTemplateID, profileID string) (*Bundle, error) {
	profileID = strings.TrimSpace(strings.ToLower(profileID))
	if profileID == "" || profileID == "clean" {
		return nil, nil
	}
	if !catalog.SoftwareAllowed(osTemplateID, profileID) {
		return nil, fmt.Errorf("software %q not allowed for os %q", profileID, osTemplateID)
	}
	if catalog.ResolveOSFamily(osTemplateID) == "windows" {
		return nil, fmt.Errorf("software preinstall is not supported on Windows")
	}
	ip = strings.TrimSpace(ip)
	password = strings.TrimSpace(password)
	if ip == "" || password == "" {
		return nil, fmt.Errorf("missing ip or password for software install")
	}

	if profileID == "3x-ui" {
		return apply3xUI(ctx, ip, password)
	}
	if profileID == "claude-code" {
		return applyClaudeCode(ctx, ip, password)
	}
	if profileID == "amnezia" {
		return applyAmnezia(ctx, ip, password)
	}

	script, ok := scripts[profileID]
	if !ok {
		return nil, fmt.Errorf("no install script for %q", profileID)
	}

	installCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		timeout := 8 * time.Minute
		if profileID == "marzban" {
			timeout = 15 * time.Minute
		}
		installCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if err := sshavail.RunScript(installCtx, ip, password, script); err != nil {
		return nil, fmt.Errorf("%s install: %w", profileID, err)
	}
	return nil, nil
}

// BundleMap converts a bundle to a JSON-friendly map for provider_meta storage.
func BundleMap(bundle *Bundle) map[string]any {
	if bundle == nil {
		return nil
	}
	b, _ := json.Marshal(bundle)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}

var scripts = map[string]string{
	"python3": python3Script,
	"marzban": marzbanScript,
}

const python3Script = `set -euo pipefail
INFO=/root/install_info.txt
{
  echo "Software: python3"
  echo "Installed at: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
} > "$INFO"
export DEBIAN_FRONTEND=noninteractive
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -y
  apt-get install -y python3 python3-pip python3-venv
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y python3 python3-pip
elif command -v yum >/dev/null 2>&1; then
  yum install -y python3 python3-pip
elif command -v pkg >/dev/null 2>&1; then
  pkg install -y python3 py39-pip || pkg install -y python311
else
  echo "Unsupported package manager" >> "$INFO"
  exit 1
fi
python3 --version >> "$INFO" 2>&1 || true
echo "python3 ready" >> "$INFO"
`

// Official Marzban installer (SQLite). Credentials → /root/info.txt
const marzbanScript = `set -euo pipefail
INFO=/root/info.txt
{
  echo "Software: Marzban Xray"
  echo "Installed at: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "Panel credentials and URL are below."
} > "$INFO"
export DEBIAN_FRONTEND=noninteractive
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -y
  apt-get install -y curl wget ca-certificates
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y curl wget ca-certificates
elif command -v yum >/dev/null 2>&1; then
  yum install -y curl wget ca-certificates
fi

set +e
bash -c "$(curl -sL https://github.com/Gozargah/Marzban-scripts/raw/master/marzban.sh)" @ install >> "$INFO" 2>&1
rc=$?
set -e

echo "" >> "$INFO"
echo "installer_exit=$rc" >> "$INFO"

ENV_FILE=""
for f in /opt/marzban/.env /opt/marzban/env /root/marzban/.env; do
  if [ -f "$f" ]; then ENV_FILE="$f"; break; fi
done

if [ -n "$ENV_FILE" ]; then
  echo "" >> "$INFO"
  echo "=== Marzban config ($ENV_FILE) ===" >> "$INFO"
  grep -E '^(SUDO_USERNAME|SUDO_PASSWORD|UVICORN_PORT|UVICORN_HOST|XRAY_JSON)=' "$ENV_FILE" >> "$INFO" 2>/dev/null || true
  PORT=$(grep -E '^UVICORN_PORT=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- | tr -d '"' | tr -d "'" | head -1)
  USER=$(grep -E '^SUDO_USERNAME=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- | tr -d '"' | tr -d "'" | head -1)
  PASS=$(grep -E '^SUDO_PASSWORD=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- | tr -d '"' | tr -d "'" | head -1)
  IP=$(hostname -I 2>/dev/null | awk '{print $1}')
  [ -z "$PORT" ] && PORT=8000
  echo "" >> "$INFO"
  echo "Dashboard: http://${IP}:${PORT}/dashboard/" >> "$INFO"
  [ -n "$USER" ] && echo "Username: $USER" >> "$INFO"
  [ -n "$PASS" ] && echo "Password: $PASS" >> "$INFO"
fi

if command -v marzban >/dev/null 2>&1; then
  echo "" >> "$INFO"
  echo "=== marzban cli ===" >> "$INFO"
  marzban status >> "$INFO" 2>&1 || true
fi

cp -f "$INFO" /root/install_info.txt 2>/dev/null || true

if [ "$rc" -ne 0 ]; then
  echo "Marzban install finished with errors — see log above" >> "$INFO"
  exit "$rc"
fi
echo "info in /root/info.txt" >> "$INFO"
`
