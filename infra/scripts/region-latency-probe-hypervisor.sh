#!/usr/bin/env bash
# Install nginx + Let's Encrypt latency probe on a regional hypervisor (DE/FI/GB).
# Run ON the hypervisor as root.
# Usage: FQDN=probe-de.cloud-hustle.com CERTBOT_EMAIL=admin@cloud-hustle.com bash region-latency-probe-hypervisor.sh
set -euo pipefail

FQDN="${FQDN:?set FQDN e.g. probe-de.cloud-hustle.com}"
CERTBOT_EMAIL="${CERTBOT_EMAIL:-admin@cloud-hustle.com}"
WEBROOT="/var/www/certbot"
SITE="/etc/nginx/sites-available/latency-probe.conf"
ENABLED="/etc/nginx/sites-enabled/latency-probe.conf"

echo "=== latency probe hypervisor: ${FQDN} ==="

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq nginx certbot python3-certbot-nginx >/dev/null

# Minimal hardening for public probe vhost
if grep -q 'server_tokens off' /etc/nginx/nginx.conf 2>/dev/null; then
  :
elif grep -q 'server_tokens' /etc/nginx/nginx.conf 2>/dev/null; then
  sed -i 's/^[[:space:]]*server_tokens.*/    server_tokens off;/' /etc/nginx/nginx.conf
else
  sed -i '/http {/a\    server_tokens off;' /etc/nginx/nginx.conf
fi

mkdir -p "$WEBROOT"
chmod 755 "$WEBROOT"

if command -v ufw >/dev/null 2>&1 && ufw status | grep -qi active; then
  ufw allow 80/tcp >/dev/null 2>&1 || true
  ufw allow 443/tcp >/dev/null 2>&1 || true
fi

cat >"$SITE" <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name ${FQDN};

    location /.well-known/acme-challenge/ {
        root ${WEBROOT};
        allow all;
    }

    location / {
        return 301 https://\$host\$request_uri;
    }
}
EOF

ln -sf "$SITE" "$ENABLED"
rm -f /etc/nginx/sites-enabled/default 2>/dev/null || true
nginx -t
systemctl enable nginx >/dev/null 2>&1 || true
systemctl restart nginx

if [[ ! -f "/etc/letsencrypt/live/${FQDN}/fullchain.pem" ]]; then
  certbot certonly --webroot -w "$WEBROOT" -d "$FQDN" \
    --non-interactive --agree-tos -m "$CERTBOT_EMAIL" \
    --preferred-challenges http
fi

cat >"$SITE" <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name ${FQDN};

    location /.well-known/acme-challenge/ {
        root ${WEBROOT};
        allow all;
    }

    location / {
        return 301 https://\$host\$request_uri;
    }
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name ${FQDN};

    ssl_certificate /etc/letsencrypt/live/${FQDN}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${FQDN}/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    location = / {
        add_header Cache-Control "no-store, no-cache, must-revalidate";
        return 204;
    }
}

server {
    listen 443 ssl default_server;
    listen [::]:443 ssl default_server;
    server_name _;
    ssl_reject_handshake on;
}
EOF

nginx -t
systemctl reload nginx

if ! crontab -l 2>/dev/null | grep -q "certbot renew"; then
  (crontab -l 2>/dev/null; echo "0 4 * * * certbot renew --quiet --deploy-hook 'systemctl reload nginx'") | crontab -
fi

echo "OK ${FQDN} probe listening on 443 (204)"
curl -s -o /dev/null -w "local_https:%{http_code}\n" "https://${FQDN}/" || true
