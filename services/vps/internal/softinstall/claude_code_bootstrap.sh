#!/usr/bin/env bash
# Claude Code + ttyd web terminal with HTTP basic auth. Prints BUNDLE_JSON: line.
set -euo pipefail

SERVER_IP="${SERVER_IP:-}"
TERMINAL_PORT="${TERMINAL_PORT:-7681}"
TERMINAL_USER="${TERMINAL_USER:-dev}"
TERMINAL_PASS="$(openssl rand -base64 18 | tr -d '/+=' | head -c 16)"

fail() {
  echo "bootstrap_error: $*" >&2
  exit 1
}

if [[ -z "$SERVER_IP" ]]; then
  SERVER_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
fi
[[ -n "$SERVER_IP" ]] || fail "SERVER_IP empty"

export DEBIAN_FRONTEND=noninteractive
if command -v apt-get >/dev/null 2>&1; then
  apt-get update -y
  apt-get install -y curl wget ca-certificates gnupg openssl python3 ca-certificates
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y curl wget ca-certificates openssl python3
elif command -v yum >/dev/null 2>&1; then
  yum install -y curl wget ca-certificates openssl python3
else
  fail "unsupported package manager"
fi

install_node20() {
  if command -v node >/dev/null 2>&1; then
    local major
    major="$(node -p "process.versions.node.split('.')[0]" 2>/dev/null || echo 0)"
    if [[ "${major:-0}" -ge 22 ]]; then
      return 0
    fi
  fi
  if command -v apt-get >/dev/null 2>&1; then
    curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
    apt-get install -y nodejs
  elif command -v dnf >/dev/null 2>&1; then
    curl -fsSL https://rpm.nodesource.com/setup_22.x | bash -
    dnf install -y nodejs
  elif command -v yum >/dev/null 2>&1; then
    curl -fsSL https://rpm.nodesource.com/setup_22.x | bash -
    yum install -y nodejs
  else
    fail "cannot install node.js"
  fi
  command -v node >/dev/null 2>&1 || fail "node missing after install"
  command -v npm >/dev/null 2>&1 || fail "npm missing after install"
}

install_ttyd() {
  if systemctl list-unit-files ttyd.service >/dev/null 2>&1; then
    systemctl disable --now ttyd.service >/dev/null 2>&1 || true
    systemctl mask ttyd.service >/dev/null 2>&1 || true
  fi
  local arch version url dest
  version="1.7.7"
  dest="/usr/local/bin/ttyd"
  if command -v ttyd >/dev/null 2>&1; then
    local current
    current="$(command -v ttyd)"
    if [[ "$current" == "$dest" || "$current" == /usr/local/bin/ttyd ]]; then
      return 0
    fi
  fi
  case "$(uname -m)" in
    x86_64 | amd64) arch="x86_64" ;;
    aarch64 | arm64) arch="aarch64" ;;
    armv7l | armv7) arch="armhf" ;;
    *) fail "unsupported arch for ttyd: $(uname -m)" ;;
  esac
  url="https://github.com/tsl0922/ttyd/releases/download/${version}/ttyd.${arch}"
  curl -fsSL "$url" -o "$dest"
  chmod +x "$dest"
  command -v "$dest" >/dev/null 2>&1 || fail "ttyd install failed"
}

install_node20
npm install -g @anthropic-ai/claude-code@latest
command -v claude >/dev/null 2>&1 || fail "claude CLI missing after npm install"

install_ttyd
TTYD_BIN="/usr/local/bin/ttyd"
[[ -x "$TTYD_BIN" ]] || fail "ttyd binary missing after install"

install_terminal_ui() {
  if command -v apt-get >/dev/null 2>&1; then
    apt-get install -y nginx apache2-utils fonts-jetbrains-mono >/dev/null 2>&1 || apt-get install -y nginx apache2-utils >/dev/null 2>&1 || true
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y nginx httpd-tools >/dev/null 2>&1 || true
  elif command -v yum >/dev/null 2>&1; then
    yum install -y nginx httpd-tools >/dev/null 2>&1 || true
  fi
  command -v nginx >/dev/null 2>&1 || fail "nginx required for Claude Code terminal UI"
  command -v htpasswd >/dev/null 2>&1 || fail "htpasswd required for Claude Code terminal auth"
}

install_terminal_ui

TERMINAL_SHARE=/usr/local/share/claude-code-terminal
TERMINAL_BACKEND_PORT=7682
LOGIN_SCRIPT=/usr/local/bin/claude-code-login.sh
mkdir -p "$TERMINAL_SHARE"

cat > "$LOGIN_SCRIPT" <<'EOF'
#!/usr/bin/env bash
export TERM=xterm-256color
export COLORTERM=truecolor
export FORCE_COLOR=1
export CLICOLOR=1
echo ""
echo "=== Claude Code Dev Environment ==="
echo "Step 1: authorize Anthropic account"
echo "  claude login"
echo "Step 2: start coding"
echo "  claude"
echo "Docs: https://docs.anthropic.com/en/docs/claude-code"
echo ""
exec bash -l
EOF
chmod +x "$LOGIN_SCRIPT"

cat > "$TERMINAL_SHARE/theme.json" <<'EOF'
{"background":"#1a2332","foreground":"#c0caf5","cursor":"#7dcfff","cursorAccent":"#1a2332","selectionBackground":"#33467c","selectionForeground":"#ffffff","black":"#151f2e","red":"#f7768e","green":"#9ece6a","yellow":"#e0af68","blue":"#7aa2f7","magenta":"#bb9af7","cyan":"#7dcfff","white":"#a9b1d6","brightBlack":"#565f89","brightRed":"#ff899d","brightGreen":"#b9f27c","brightYellow":"#ffc777","brightBlue":"#9abdf5","brightMagenta":"#d2a6ef","brightCyan":"#a3daff","brightWhite":"#ffffff"}
EOF

cat > "$TERMINAL_SHARE/enhance.js" <<'EOF'
(function () {
  var __claudeTheme = "v3-clipboard";

  function copyText(text) {
    if (!text) return;
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).catch(function () {
        copyTextFallback(text);
      });
      return;
    }
    copyTextFallback(text);
  }

  function copyTextFallback(text) {
    var ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.left = "-9999px";
    document.body.appendChild(ta);
    ta.select();
    try {
      document.execCommand("copy");
    } catch (e) {}
    document.body.removeChild(ta);
  }

  function patch(term) {
    if (!term || term.__claudeEnhanced) return false;
    term.__claudeEnhanced = true;
    term.options.rightClickSelectsWord = false;

    term.attachCustomKeyEventHandler(function (ev) {
      if (ev.type !== "keydown") return true;
      if (!(ev.ctrlKey || ev.metaKey) || ev.altKey) return true;
      var key = ev.key.toLowerCase();

      if (key === "c") {
        var sel = term.getSelection();
        if (sel && sel.length > 0) {
          copyText(sel);
          term.clearSelection();
          ev.preventDefault();
          return false;
        }
        return true;
      }

      if (key === "v") {
        // Let the browser emit a paste event (works on HTTP, unlike navigator.clipboard).
        return false;
      }

      return true;
    });

    var container = term.element && term.element.parentElement;
    if (container) {
      container.addEventListener("contextmenu", function (e) {
        e.preventDefault();
        term.focus();
      });
    }

    document.body.style.backgroundColor = "#1a2332";
    document.documentElement.style.backgroundColor = "#1a2332";
    return true;
  }

  function tryPatch() {
    return !!(window.term && patch(window.term));
  }

  if (!tryPatch()) {
    var tries = 0;
    var bootTimer = setInterval(function () {
      if (tryPatch() || ++tries > 400) {
        clearInterval(bootTimer);
      }
    }, 50);
  }

  var lastTerm = window.term;
  setInterval(function () {
    if (window.term && window.term !== lastTerm) {
      patch(window.term);
      lastTerm = window.term;
    }
  }, 1000);
})();
EOF

cat > /usr/local/bin/claude-code-ttyd-start.sh <<EOF
#!/usr/bin/env bash
set -euo pipefail
THEME=\$(tr -d '\n' < ${TERMINAL_SHARE}/theme.json)
exec ${TTYD_BIN} \\
  --interface 127.0.0.1 \\
  --port ${TERMINAL_BACKEND_PORT} \\
  -T xterm-256color \\
  -W \\
  -t disableLeaveAlert=true \\
  -t disableResizeOverlay=true \\
  -t fontSize=14 \\
  -t lineHeight=1.35 \\
  -t cursorBlink=true \\
  -t cursorStyle=bar \\
  -t scrollback=10000 \\
  -t drawBoldTextInBrightColors=true \\
  -t 'fontFamily=JetBrains Mono, Fira Code, Cascadia Code, Menlo, monospace' \\
  -t "theme=\${THEME}" \\
  ${LOGIN_SCRIPT}
EOF
chmod +x /usr/local/bin/claude-code-ttyd-start.sh

htpasswd -bc /etc/nginx/.claude-terminal-htpasswd "${TERMINAL_USER}" "${TERMINAL_PASS}" >/dev/null 2>&1

cat > /etc/nginx/sites-available/claude-code-terminal <<EOF
server {
    listen ${TERMINAL_PORT} default_server;
    listen [::]:${TERMINAL_PORT} default_server;
    server_name _;

    auth_basic "Claude Code Terminal";
    auth_basic_user_file /etc/nginx/.claude-terminal-htpasswd;

    location = /claude-terminal.js {
        alias ${TERMINAL_SHARE}/enhance.js;
        default_type application/javascript;
        auth_basic off;
        add_header Cache-Control "no-store";
    }

    location / {
        proxy_pass http://127.0.0.1:${TERMINAL_BACKEND_PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header Accept-Encoding "";
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        sub_filter '</body>' '<script src="/claude-terminal.js"></script></body>';
        sub_filter_once on;
    }
}
EOF

if [[ -d /etc/nginx/sites-enabled ]]; then
  ln -sf /etc/nginx/sites-available/claude-code-terminal /etc/nginx/sites-enabled/claude-code-terminal
  rm -f /etc/nginx/sites-enabled/default
  rm -f /etc/nginx/conf.d/claude-code-terminal.conf
elif [[ -d /etc/nginx/conf.d ]]; then
  cp /etc/nginx/sites-available/claude-code-terminal /etc/nginx/conf.d/claude-code-terminal.conf
fi

cat > /etc/systemd/system/claude-code-ttyd.service <<EOF
[Unit]
Description=Claude Code web terminal backend (ttyd)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/claude-code-ttyd-start.sh
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable claude-code-ttyd nginx >/dev/null 2>&1 || systemctl enable claude-code-ttyd >/dev/null 2>&1 || true
systemctl restart claude-code-ttyd >/dev/null 2>&1 || fail "claude-code-ttyd failed to start"
if ! nginx -t >/tmp/claude-nginx-test.log 2>&1; then
  fail "nginx config invalid: $(tr '\n' ' ' </tmp/claude-nginx-test.log | tail -c 400)"
fi
systemctl restart nginx >/dev/null 2>&1 || fail "nginx failed to start"

for _wait in $(seq 1 30); do
  if ss -tln 2>/dev/null | grep -qE "0\\.0\\.0\\.0:${TERMINAL_PORT} |\\*:${TERMINAL_PORT} |\\[::\\]:${TERMINAL_PORT} " \
    || netstat -tln 2>/dev/null | grep -qE "0\\.0\\.0\\.0:${TERMINAL_PORT} |\\*:${TERMINAL_PORT} "; then
    break
  fi
  sleep 1
done
if ! ss -tln 2>/dev/null | grep -qE "0\\.0\\.0\\.0:${TERMINAL_PORT} |\\*:${TERMINAL_PORT} |\\[::\\]:${TERMINAL_PORT} " \
  && ! netstat -tln 2>/dev/null | grep -qE "0\\.0\\.0\\.0:${TERMINAL_PORT} |\\*:${TERMINAL_PORT} "; then
  fail "ttyd port ${TERMINAL_PORT} not listening on public interface"
fi
if ! systemctl is-active --quiet claude-code-ttyd; then
  fail "claude-code-ttyd service not active"
fi
if ! systemctl is-active --quiet nginx; then
  fail "nginx terminal proxy not active"
fi

if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi active; then
  ufw allow "${TERMINAL_PORT}/tcp" >/dev/null 2>&1 || true
fi

PANEL_URL="http://${SERVER_IP}:${TERMINAL_PORT}/"
python3 - <<PY
import json
bundle = {
    "profile": "claude-code",
    "panel": {
        "url": ${PANEL_URL@Q},
        "username": ${TERMINAL_USER@Q},
        "password": ${TERMINAL_PASS@Q},
    },
    "claude": {
        "login_hint": "In the web terminal run: claude login",
        "start_command": "claude",
    },
}
print("BUNDLE_JSON:" + json.dumps(bundle, ensure_ascii=False))
PY
