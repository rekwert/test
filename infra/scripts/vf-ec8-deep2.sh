#!/bin/bash
source /opt/virtfusion/app/control/.env
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== NEW ALLOCATION LOGS (last hour) ==="
myG "SELECT id,success,message,accepted,rejected,created_at FROM log_resource_allocation ORDER BY id DESC LIMIT 15;"

echo "=== LOG FULL MESSAGE if column exists ==="
myG "DESCRIBE log_resource_allocation;" | head -20

echo "=== HYPERVISOR CONNECTION MONITOR ==="
myG "SELECT * FROM sys_mon_hypervisor\G" 2>/dev/null
myG "SHOW TABLES LIKE '%mon%';"

echo "=== TRY CONTROL->HV CALL via PHP ==="
/opt/virtfusion/php8/bin/php <<'PHP'
<?php
require '/opt/virtfusion/app/control/vendor/autoload.php';
$app = require '/opt/virtfusion/app/control/bootstrap/app.php';
$app->make('Illuminate\Contracts\Console\Kernel')->bootstrap();

try {
    $hv = App\Models\Hypervisor::find(1);
    // common VF patterns
    foreach (['connectionStatus','getConnectionStatus','isConnected','testConnection','resources','getResources','resourceData'] as $m) {
        if (method_exists($hv, $m)) {
            echo "Method $m exists\n";
            try {
                $r = $hv->$m();
                echo "$m => ".json_encode($r)."\n";
            } catch (Throwable $e) {
                echo "$m ERR: ".$e->getMessage()."\n";
            }
        }
    }
    // Try job dispatch
    if (class_exists('App\\Jobs\\Hypervisor\\UpdateHypervisorResources')) {
        echo "Dispatching UpdateHypervisorResources\n";
        App\Jobs\Hypervisor\UpdateHypervisorResources::dispatch(1);
    }
    if (class_exists('App\\Jobs\\Hypervisor\\MonitorHypervisor')) {
        echo "Dispatching MonitorHypervisor\n";
        App\Jobs\Hypervisor\MonitorHypervisor::dispatch(1);
    }
} catch (Throwable $e) {
    echo "ERR: ".$e->getMessage()."\n";
}
PHP

sleep 5

echo "=== HV APP LOG LAST 10 lines ==="
tail -10 /opt/virtfusion/app/hypervisor/storage/logs/app-$(date +%Y-%m-%d).log 2>/dev/null

echo "=== LICENSE TYPE MIGRATION ==="
grep -A5 "license_type" /opt/virtfusion/app/control/database/migrations/2021_03_24_204013_add_license_type_to_hypervisors_table.php 2>/dev/null

echo "=== CONFIG license related all keys ==="
myG "SELECT \`key\` FROM configuration WHERE \`key\` LIKE '%licen%' OR \`key\` LIKE '%eval%' OR \`key\` LIKE '%server%limit%' OR \`key\` LIKE '%hypervisor%';"
