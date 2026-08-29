#!/bin/bash
set -euo pipefail
export PATH="/snap/bin:${PATH}"
OUT="/tmp/sunbeam-prepare-$$.sh"
sunbeam prepare-node-script --bootstrap > "$OUT"
bash -x "$OUT"
rm -f "$OUT"
