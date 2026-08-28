#!/usr/bin/env bash
set -u
source /opt/testVPStrade/infra/docker/.env.probe
SSHPASS="$NL_SSH_PASS" sshpass -e ssh -o StrictHostKeyChecking=no root@66.248.206.14 bash -s <<'NL'
set -u
cd /opt/virtfusion/app/control
PHP=/opt/virtfusion/php8/bin/php

echo "=== artisan commands ==="
$PHP artisan list --raw 2>&1 | python3 -c 'import sys; print("".join(x for x in sys.stdin if "commission" in x.lower() or "hypervisor" in x.lower()), end="")'
echo "list_status=$?"

echo "=== help ==="
$PHP artisan hypervisor:re-commission --help 2>&1
echo "help_status=$?"

echo "=== source files ==="
FILES=$(python3 <<'PY'
import os
for root in ("app", "routes", "vendor"):
    if not os.path.isdir(root):
        continue
    for base, dirs, files in os.walk(root):
        dirs[:] = [d for d in dirs if d not in ("node_modules", ".git")]
        for name in files:
            if not name.endswith(".php"):
                continue
            path = os.path.join(base, name)
            try:
                text = open(path, errors="ignore").read()
            except OSError:
                continue
            if "hypervisor:re-commission" in text:
                print(path)
PY
)
echo "$FILES"
for F in $FILES; do
  echo "=== FILE $F ==="
  python3 - "$F" <<'PY'
import sys
p = sys.argv[1]
for i, line in enumerate(open(p, errors="replace"), 1):
    if "commission" in line.lower() or "ask(" in line or "choice(" in line or "confirm(" in line or "secret(" in line:
        lo=max(1,i-8); hi=i+15
        print(f"-- {lo}:{hi} --")
        for n,s in enumerate(open(p, errors="replace"),1):
            if lo <= n <= hi:
                print(f"{n}: {s}", end="")
PY
done

echo "=== command reflection ==="
$PHP <<'PHP'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$kernel = $app->make(Illuminate\Contracts\Console\Kernel::class);
$kernel->bootstrap();
$all = Illuminate\Support\Facades\Artisan::all();
$cmd = $all['hypervisor:re-commission'];
$r = new ReflectionClass($cmd);
echo "class=" . $r->getName() . PHP_EOL;
echo "file=" . $r->getFileName() . PHP_EOL;
foreach (['handle', 'fire', 'execute', 'interact'] as $method) {
    if ($r->hasMethod($method)) {
        $m = $r->getMethod($method);
        echo "{$method}={$m->getStartLine()}:{$m->getEndLine()}" . PHP_EOL;
    }
}
PHP

echo "=== reflected source ==="
python3 - <<'PY'
p = "/opt/virtfusion/app/control/app/Console/Commands/HypervisorRecommission.php"
for n, line in enumerate(open(p, errors="replace"), 1):
    if n <= 100:
        print(f"{n}: {line}", end="")
PY
NL
