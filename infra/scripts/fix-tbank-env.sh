#!/usr/bin/env bash
set -euo pipefail
exec python3 - "$@" <<'PY'
import re, sys
path = sys.argv[1] if len(sys.argv) > 1 else "/opt/testVPStrade/infra/docker/.env"
text = open(path, encoding="utf-8").read()
text = re.sub(r"^TBANK_PASSWORD=.*$", "TBANK_PASSWORD=a%cn2DY$$EPIef6IE", text, flags=re.M)
open(path, "w", encoding="utf-8").write(text)
print("TBANK_PASSWORD restored for docker compose --env-file")
PY