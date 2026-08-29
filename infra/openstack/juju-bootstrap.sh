#!/bin/bash
set -euo pipefail
export PATH="/snap/bin:${PATH}"
exec juju bootstrap localhost
