#!/bin/bash
source /opt/virtfusion/app/control/.env
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== AUTH.JSON ==="
ls -la /opt/virtfusion/app/hypervisor/conf/ 2>/dev/null

echo "=== COMMISSIONED ==="
myG "SELECT commissioned,enabled FROM hypervisors WHERE id=1;"

echo "=== LICENSE KEYS IN ALL TABLES ==="
myG "SELECT TABLE_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='$DB_DATABASE' AND COLUMN_NAME LIKE '%license%';"

echo "=== SEARCH licenseKey ==="
myG "SELECT TABLE_NAME,COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='$DB_DATABASE' AND (COLUMN_NAME LIKE '%license%' OR COLUMN_NAME='key');" | head -30

echo "=== system health ==="
myG "SELECT * FROM system_health_settings;"

echo "=== access settings ==="
myG "SELECT * FROM access_settings;"

echo "=== PHP license check ==="
/opt/virtfusion/php8/bin/php -r '
require "/opt/virtfusion/app/control/vendor/autoload.php";
$app=require "/opt/virtfusion/app/control/bootstrap/app.php";
$app->make("Illuminate\Contracts\Console\Kernel")->bootstrap();
foreach (["licenseKey","licenseLast","licenseType","licenseStatus"] as $k) {
  $v = DB::table("sys_settings")->where("key",$k)->value("value");
  if ($v!==null) echo "$k=$v\n";
}
' 2>&1

echo "=== ACCEPT CHECK from back will be separate ==="
