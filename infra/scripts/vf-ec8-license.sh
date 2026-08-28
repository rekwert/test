#!/bin/bash
source /opt/virtfusion/app/control/.env
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== LICENSE CONFIG ==="
myG "SELECT \`key\`, LEFT(value,80) val FROM configuration WHERE \`key\` LIKE 'license%';"

echo "=== HYPERVISOR ==="
myG "SELECT id,name,enabled,commissioned,license_type,max_servers,prohibit,maintenance FROM hypervisors\G"

echo "=== GROUP RESOURCES via local test ==="
PHP=/opt/virtfusion/php8/bin/php
cd /opt/virtfusion/app/control
$PHP -r '
require "vendor/autoload.php";
$app=require "bootstrap/app.php";
$app->make("Illuminate\Contracts\Console\Kernel")->bootstrap();
$rows = DB::table("configuration")->where("key","like","license%")->get(["key","value"]);
foreach ($rows as $r) echo $r->key."=".substr($r->value,0,60)."\n";
' 2>&1 | head -20

echo "=== PACKAGE ==="
myG "SELECT id,name,enabled,memory,storage,cpu_cores FROM server_packages WHERE id=1\G"

echo "=== RECENT ALLOCATION LOG ==="
myG "SELECT id,success,message,accepted,rejected,created_at FROM log_resource_allocation ORDER BY id DESC LIMIT 5;"

echo "=== EVAL LICENSE TYPE ==="
myG "SELECT license_type, COUNT(*) c FROM hypervisors GROUP BY license_type;"

# license_type meanings: check migration
grep -r "license_type" /opt/virtfusion/app/control/database/migrations/ 2>/dev/null | head -5

echo "=== SERVERS COUNT ==="
myG "SELECT COUNT(*) FROM servers;"

echo "=== LIBVIRT ==="
systemctl is-active libvirtd
virsh list --all 2>/dev/null | head -5
