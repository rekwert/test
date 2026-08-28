#!/bin/bash
source /opt/virtfusion/app/control/.env
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== SYS MON HYPERVISOR ==="
myG "SELECT * FROM sys_mon_hypervisor\G"

echo "=== ALLOCATION 7335 DETAIL ==="
myG "SELECT id,message,success,resources,allocation FROM log_resource_allocation WHERE id=7335\G"

echo "=== AUTH.JSON ==="
cat /opt/virtfusion/app/hypervisor/conf/auth.json

echo ""
echo "=== HYPERVISOR TOKEN PREFIX (DB) ==="
myG "SELECT id, LEFT(token,80) tok FROM hypervisors WHERE id=1;"

echo "=== CONTROL CONNECT TEST (curl admin internal) ==="
# simulate control -> hypervisor with Laravel HTTP client via php
/opt/virtfusion/php8/bin/php <<'PHP'
<?php
require '/opt/virtfusion/app/control/vendor/autoload.php';
$app = require '/opt/virtfusion/app/control/bootstrap/app.php';
$app->make('Illuminate\Contracts\Console\Kernel')->bootstrap();
$hv = App\Models\Hypervisor::find(1);
echo "IP: {$hv->ip} Port: {$hv->port}\n";
// Search for HypervisorAPI / Client classes
foreach (get_declared_classes() as $cls) {
    if (stripos($cls, 'Hypervisor') !== false && stripos($cls, 'Virtfusion') !== false) {}
}
$classes = [
    'App\\Services\\Hypervisor\\HypervisorConnection',
    'App\\Library\\Hypervisor\\Connection',
    'App\\Core\\Hypervisor\\HypervisorConnectionService',
    'App\\Models\\Hypervisor\\Connection',
];
foreach ($classes as $c) {
    if (class_exists($c)) echo "FOUND $c\n";
}
// List app bindings containing hypervisor
foreach ($app->getBindings() as $k => $v) {
    if (stripos($k, 'hypervisor') !== false) echo "BINDING: $k\n";
}
PHP

echo "=== UFW STATUS ==="
ufw status 2>/dev/null | head -10 || echo no-ufw

echo "=== FIX UFW routed (VF FAQ) ==="
ufw default allow routed 2>/dev/null || true
