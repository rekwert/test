#!/bin/bash
set -a
source /opt/virtfusion/app/control/.env 2>/dev/null
set +a
PHP=/opt/virtfusion/php8/bin/php

echo "=== PHP ARTISAN ==="
cd /opt/virtfusion/app/control
$PHP artisan list 2>/dev/null | grep -iE 'hypervisor|license|resource|commission|server' | head -50

echo "=== LICENSE FILES ==="
find /opt/virtfusion -name '*license*' 2>/dev/null | head -20
grep -r 'licenseKey\|VFEVL' /opt/virtfusion/app/control/storage/ /opt/virtfusion/app/control/.env 2>/dev/null | head -10

echo "=== SS SETTINGS ==="
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT * FROM ss_settings;" 2>/dev/null

echo "=== VIRTUALIZATION SETTINGS ==="
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT * FROM virtualization_settings;" 2>/dev/null

echo "=== SERVERS SETTINGS ==="
mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "SELECT * FROM servers_settings;" 2>/dev/null

echo "=== QUEUE HV FULL TAIL ==="
supervisorctl tail -3000 vf-queue-hv:00 2>/dev/null | tail -80

echo "=== CONTROL .env LICENSE ==="
grep -i license /opt/virtfusion/app/control/.env 2>/dev/null

echo "=== TRY DECRYPT TOKEN AND CALL AGENT ==="
$PHP -r '
require "/opt/virtfusion/app/control/vendor/autoload.php";
$app = require "/opt/virtfusion/app/control/bootstrap/app.php";
$app->make("Illuminate\Contracts\Console\Kernel")->bootstrap();
$h = DB::table("hypervisors")->where("id",1)->first();
echo "hv id=".$h->id." commissioned=".$h->commissioned."\n";
' 2>&1 | head -20
