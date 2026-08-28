#!/usr/bin/env bash
# Run ON VirtFusion panel (66.248.206.14) as root.
# Injects autounattend.xml into Win11 template qcow2 and re-downloads to hypervisors.
#
# From back server:
#   NL_PASS='...' bash infra/scripts/vf-remote-panel.sh vf-win11-inject-unattend.sh
set -euo pipefail

source /opt/virtfusion/app/control/.env
PHP=/opt/virtfusion/php8/bin/php
VFCTL=/opt/virtfusion/app/control
MY=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE")
MYN=(mysql -h127.0.0.1 -u"$DB_USERNAME" -p"$DB_PASSWORD" "$DB_DATABASE" -N)

TPL_DIR=/home/vf-data/os/template
ISO_DIR=/var/www/html/iso
TEMPLATE_ID=23
UNATTEND_SRC="${UNATTEND_SRC:-/root/autounattend.xml}"

if [[ ! -f "$UNATTEND_SRC" ]]; then
  echo "missing $UNATTEND_SRC — copy infra/windows/autounattend.xml to panel first" >&2
  exit 1
fi

pick_img() {
  for f in \
    "$TPL_DIR/windows11_cloudbase_60g.qcow2" \
    "$TPL_DIR/windows11_cloudbase.qcow2" \
    "$ISO_DIR/windows11-cloudbase-60g.qcow2" \
    "$ISO_DIR/windows11-cloudbase.img"; do
    if [[ -f "$f" ]]; then
      echo "$f"
      return
    fi
  done
  echo "no Win11 qcow2/img under $TPL_DIR or $ISO_DIR" >&2
  exit 1
}

if ! command -v virt-customize >/dev/null 2>&1; then
  echo "installing libguestfs-tools..."
  apt-get update -qq && apt-get install -y -qq libguestfs-tools
fi

IMG=$(pick_img)
STAMP=$(date +%Y%m%d%H%M%S)
BACKUP="${IMG}.pre-unattend.${STAMP}"
echo "=== backup $IMG -> $BACKUP ==="
cp -a "$IMG" "$BACKUP"

echo "=== inject unattend into $IMG ==="
virt-customize -a "$IMG" \
  --upload "$UNATTEND_SRC:/Windows/System32/Sysprep/unattend.xml"

echo "=== refresh ISO mirrors ==="
for dest in \
  "$TPL_DIR/windows11_cloudbase_60g.qcow2" \
  "$ISO_DIR/windows11-cloudbase-60g.qcow2" \
  "$ISO_DIR/windows11-cloudbase.img"; do
  if [[ "$dest" != "$IMG" ]]; then
    cp -f "$IMG" "$dest" 2>/dev/null || true
    chmod 644 "$dest" 2>/dev/null || true
  fi
done
chmod 644 "$IMG"

echo "=== qemu-img info ==="
qemu-img info "$IMG" | head -8

echo "=== clear stale os_tpl_download for template $TEMPLATE_ID ==="
if "${MY[@]}" -e "DESCRIBE os_tpl_download;" &>/dev/null; then
  if "${MY[@]}" -e "SHOW COLUMNS FROM os_tpl_download LIKE 'template_id';" 2>/dev/null | grep -q template_id; then
    "${MY[@]}" -e "DELETE FROM os_tpl_download WHERE template_id=$TEMPLATE_ID;"
  else
    "${MY[@]}" -e "DELETE FROM os_tpl_download WHERE os_tpl_id=$TEMPLATE_ID;" 2>/dev/null || true
  fi
fi

echo "=== re-commission hypervisors with Win11 template ==="
cd "$VFCTL"
mapfile -t HVS < <("${MYN[@]}" -e "
SELECT id FROM hypervisors
WHERE enabled=1 AND commissioned=1
ORDER BY id;" 2>/dev/null || echo -e "1\n2\n3\n4\n5")

for HV in "${HVS[@]}"; do
  HV_NAME=$("${MYN[@]}" -e "SELECT name FROM hypervisors WHERE id=$HV;" 2>/dev/null || echo "?")
  echo ">>> hypervisor:re-commission $HV ($HV_NAME)"
  printf '%s\nyes\nyes\n' "$HV" | "$PHP" artisan hypervisor:re-commission 2>&1 | tail -20 || true
  sleep 6
done

echo "=== os_tpl_download status (template $TEMPLATE_ID) ==="
if "${MY[@]}" -e "DESCRIBE os_tpl_download;" &>/dev/null; then
  if "${MY[@]}" -e "SHOW COLUMNS FROM os_tpl_download LIKE 'template_id';" 2>/dev/null | grep -q template_id; then
    "${MY[@]}" -e "
SELECT d.hypervisor_id, h.name, d.completed
FROM os_tpl_download d
JOIN hypervisors h ON h.id = d.hypervisor_id
WHERE d.template_id=$TEMPLATE_ID
ORDER BY d.hypervisor_id;" 2>/dev/null || true
  fi
fi

echo VF_WIN11_INJECT_UNATTEND_DONE