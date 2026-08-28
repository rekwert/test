#!/bin/bash
source /opt/virtfusion/app/control/.env
myG() { mysql -h"$DB_HOST" -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -e "$1" 2>/dev/null; }

echo "=== CONFIGURATION TABLE ==="
myG "SELECT * FROM configuration WHERE \`key\` LIKE '%licen%' OR \`key\` LIKE '%trial%' OR \`key\` LIKE '%limit%';"
myG "SELECT * FROM system WHERE \`key\` LIKE '%licen%' OR \`key\` LIKE '%trial%';"

echo "=== WIDEN API TOKEN ACCESS ==="
myG "UPDATE global_api_tokens SET access='[\"*\"]' WHERE id=1;"
myG "SELECT id,access FROM global_api_tokens;"

echo "=== DOWNLOAD DEFAULT OS TEMPLATES? ==="
myG "SELECT * FROM configuration WHERE \`key\` LIKE '%os%' OR \`key\` LIKE '%template%' OR \`key\` LIKE '%media%';" | head -20
