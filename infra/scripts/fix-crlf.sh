#!/bin/bash
set -e
for f in "$@"; do
  python3 - "$f" <<'PY'
import sys
p = sys.argv[1]
data = open(p, "rb").read()
data = data.replace(b"\r\n", b"\n").replace(b"\r", b"\n")
open(p, "wb").write(data)
print("fixed:", p)
PY
done
