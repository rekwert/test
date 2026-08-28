#!/bin/bash
PHP=/opt/virtfusion/php8/bin/php
cd /opt/virtfusion/app/control

$PHP <<'PHP'
<?php
require __DIR__ . '/vendor/autoload.php';
$app = require __DIR__ . '/bootstrap/app.php';
$kernel = $app->make(Illuminate\Contracts\Console\Kernel::class);
$kernel->bootstrap();

try {
    $hv = DB::table('hypervisors')->where('id', 1)->first();
    echo "commissioned={$hv->commissioned} enabled={$hv->enabled}\n";
    
    // Try common VF hypervisor client classes
    foreach ([
        'App\\Services\\Hypervisor\\HypervisorService',
        'App\\Core\\Hypervisor\\Hypervisor',
        'App\\Models\\Hypervisor',
    ] as $cls) {
        if (class_exists($cls)) echo "CLASS EXISTS: $cls\n";
    }
    
    $model = app('App\\Models\\Hypervisor');
    $h = $model::find(1);
    if ($h) {
        echo "Model found: {$h->name}\n";
        if (method_exists($h, 'getResources')) {
            print_r($h->getResources());
        }
        if (method_exists($h, 'resources')) {
            print_r($h->resources());
        }
        if (method_exists($h, 'updateResources')) {
            $h->updateResources();
            echo "updateResources called\n";
        }
    }
} catch (Throwable $e) {
    echo "ERR: ".$e->getMessage()."\n";
    echo $e->getFile().':'.$e->getLine()."\n";
}
PHP

echo "=== TRY COMMISSION VIA ARTISAN HELP ==="
$PHP artisan hypervisor:re-commission --help 2>&1 | head -20

echo "=== REMOVE AUTH AND RECOMMISSION ==="
read -r -d '' _ # noop
# backup auth
cp /opt/virtfusion/app/hypervisor/conf/auth.json /opt/virtfusion/app/hypervisor/conf/auth.json.bak.$(date +%s) 2>/dev/null || true
rm -f /opt/virtfusion/app/hypervisor/conf/auth.json
myG() { mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }
source /opt/virtfusion/app/control/.env
myG "UPDATE hypervisors SET commissioned=0 WHERE id=1;"
printf '1\nyes\nyes\n' | $PHP artisan hypervisor:re-commission 2>&1 | tail -15
sleep 5
ls -la /opt/virtfusion/app/hypervisor/conf/auth.json 2>/dev/null || echo "auth.json not recreated yet"
myG "SELECT commissioned FROM hypervisors WHERE id=1;"
