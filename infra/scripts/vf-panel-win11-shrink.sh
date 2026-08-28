#!/usr/bin/env bash
# Run ON VirtFusion panel (66.248.206.14): shrink Win11 template in place + sync GB.
set -euo pipefail

source /opt/virtfusion/app/control/.env
PHP=/opt/virtfusion/php8/bin/php
VFCTL=/opt/virtfusion/app/control
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")

TPL_DIR=/home/vf-data/os/template
ISO_DIR=/var/www/html/iso
SRC="${TPL_DIR}/windows11_cloudbase.qcow2"
ISO="${ISO_DIR}/windows11-cloudbase.img"
NEW="${TPL_DIR}/windows11_cloudbase_60g.qcow2"
NEW_ISO="${ISO_DIR}/windows11-cloudbase-60g.qcow2"
TEMPLATE_ID=23

pick_src() {
  if [[ -f "$SRC" ]]; then echo "$SRC"; return; fi
  if [[ -f "$ISO" ]]; then echo "$ISO"; return; fi
  echo "missing Win11 image under $TPL_DIR or $ISO_DIR" >&2
  exit 1
}

SRC_FILE=$(pick_src)
echo "=== source: $SRC_FILE ==="
qemu-img info "$SRC_FILE" | head -8

if [[ ! -f "$NEW" ]]; then
  echo "=== convert + shrink to 60G ==="
  qemu-img convert -O qcow2 "$SRC_FILE" "$NEW"
  qemu-img resize --shrink "$NEW" 60G
fi

echo "=== shrunk ==="
qemu-img info "$NEW" | head -8

cp -f "$NEW" "$NEW_ISO"
chmod 644 "$NEW" "$NEW_ISO"

echo "=== update media_templates id=$TEMPLATE_ID ==="
"${MY[@]}" -e "UPDATE media_templates SET filename='windows11_cloudbase_60g.qcow2', url='https://panel.cloud-hustle.com/iso/windows11-cloudbase-60g.qcow2', updated_at=NOW() WHERE id=$TEMPLATE_ID;"

echo "=== clear stale os_tpl_download for template $TEMPLATE_ID ==="
if "${MY[@]}" -e "DESCRIBE os_tpl_download;" &>/dev/null; then
  "${MY[@]}" -e "DELETE FROM os_tpl_download WHERE os_tpl_id=$TEMPLATE_ID;"
fi

echo "=== re-commission GB hypervisor 4 ==="
cd "$VFCTL"
printf '4\nyes\nyes\n' | "$PHP" artisan hypervisor:re-commission 2>&1 | tail -30

echo "=== os_tpl_download GB ==="
if "${MY[@]}" -e "DESCRIBE os_tpl_download;" &>/dev/null; then
  "${MY[@]}" -e "SELECT hypervisor_id, os_tpl_id, status FROM os_tpl_download WHERE hypervisor_id=4 AND os_tpl_id=$TEMPLATE_ID;"
fi

echo VF_PANEL_WIN11_SHRINK_DONE
