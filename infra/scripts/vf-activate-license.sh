#!/bin/bash
source /opt/virtfusion/app/control/.env
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== LICENSE IN DB ==="
myG "SELECT \`key\`, LEFT(value,100) val FROM configuration WHERE \`key\` LIKE 'license%' ORDER BY \`key\`;"

echo ""
echo "=== ARTISAN LICENSE ==="
/opt/virtfusion/php8/bin/php /opt/virtfusion/app/control/artisan list 2>/dev/null | grep -i license || echo "(no license artisan cmd)"

echo ""
echo "=== TRY ACTIVATE NEW KEY ==="
NEWKEY="VFSTL-7QTKF-4RSKG-MV73T-KU7YV"
/opt/virtfusion/php8/bin/php /opt/virtfusion/app/control/artisan 2>&1 | head -3
for cmd in "license:activate $NEWKEY" "license:apply $NEWKEY" "license:update $NEWKEY"; do
  echo "try: $cmd"
  /opt/virtfusion/php8/bin/php /opt/virtfusion/app/control/artisan $cmd 2>&1 | head -5
done
