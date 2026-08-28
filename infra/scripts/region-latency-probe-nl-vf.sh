#!/usr/bin/env bash
# Add /latency-probe to VirtFusion panel nginx (NL). Uses existing panel TLS cert.
# Run ON NL panel host (66.248.206.14) as root.
set -euo pipefail

UI="/opt/virtfusion/nginx/conf/conf.d/ui.conf"

if grep -q 'location = /latency-probe' "$UI" 2>/dev/null; then
  echo "NL /latency-probe already configured in ui.conf"
else
  cp -a "$UI" "${UI}.bak.latency-probe.$(date +%Y%m%d%H%M%S)"
  sed -i '/location = \/favicon.ico/i\
          location = /latency-probe {\
                add_header Cache-Control "no-store, no-cache, must-revalidate";\
                return 204;\
          }\
' "$UI"
  echo "Inserted /latency-probe into $UI"
fi

/opt/virtfusion/nginx/sbin/nginx -t
/opt/virtfusion/nginx/sbin/nginx -s reload

code=$(curl -sk -o /dev/null -w '%{http_code}' https://127.0.0.1/latency-probe -H 'Host: panel.cloud-hustle.com')
echo "local_probe_http_code=${code}"
