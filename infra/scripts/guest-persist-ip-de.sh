#!/usr/bin/env bash
# Run on DE hypervisor. Args: VM_UUID NEW_IP [GW] [PFX]
set -euo pipefail
vm=${1:?uuid}
NEW=${2:?ip}
GW=${3:-212.102.227.1}
PFX=${4:-24}

if ! virsh domstate "$vm" 2>/dev/null | grep -q running; then echo skip:not_running; exit 0; fi
if ! virsh qemu-agent-command "$vm" '{"execute":"guest-ping"}' >/dev/null 2>&1; then echo skip:no_agent; exit 1; fi

guest=$(cat <<SCRIPT
set -e
IFACE=\$(ip -o link show | awk -F': ' '\$2!~/lo/{print \$2; exit}')
test -n "\$IFACE"
NEW='$NEW'
GW='$GW'
PFX='$PFX'
mkdir -p /etc/network/interfaces.d /etc/cloud/cloud.cfg.d
echo 'network: {config: disabled}' > /etc/cloud/cloud.cfg.d/99-disable-network-config.cfg
cat > /etc/network/interfaces <<'EOF'
auto lo
iface lo inet loopback
source /etc/network/interfaces.d/*
EOF
rm -f /etc/network/interfaces.d/*
cat > /etc/network/interfaces.d/50-static <<EOF
auto \$IFACE
iface \$IFACE inet static
    address \$NEW/\$PFX
    gateway \$GW
    dns-nameservers 1.1.1.1 8.8.8.8
EOF
if ls /etc/netplan/*.yaml >/dev/null 2>&1; then
  rm -f /etc/netplan/50-cloud-init.yaml 2>/dev/null || true
  cat > /etc/netplan/50-static.yaml <<EOF
network:
  version: 2
  ethernets:
    \$IFACE:
      addresses: [\$NEW/\$PFX]
      routes:
        - to: default
          via: \$GW
      nameservers:
        addresses: [1.1.1.1, 8.8.8.8]
EOF
  netplan apply 2>/dev/null || true
fi
ip addr flush dev "\$IFACE" 2>/dev/null || true
ip addr add "\$NEW/\$PFX" dev "\$IFACE" 2>/dev/null || ip addr replace "\$NEW/\$PFX" dev "\$IFACE"
ip route replace default via "\$GW" dev "\$IFACE"
echo LIVE:\$(ip -4 -o addr show dev "\$IFACE" | awk '{print \$4}')
test -f /etc/netplan/50-static.yaml && grep -q "\$NEW" /etc/netplan/50-static.yaml && echo NETPLAN_OK
SCRIPT
)

b64=$(printf '%s' "$guest" | base64 -w0)
payload=$(python3 -c "import json,sys; b=sys.argv[1]; print(json.dumps({'execute':'guest-exec','arguments':{'path':'/bin/bash','arg':['-c','echo '+b+' | base64 -d | bash'],'capture-output':True}}))" "$b64")
pid=$(virsh qemu-agent-command "$vm" "$payload" | python3 -c "import json,sys; print(json.load(sys.stdin)['return']['pid'])")
sleep 4
virsh qemu-agent-command "$vm" "{\"execute\":\"guest-exec-status\",\"arguments\":{\"pid\":$pid}}" | python3 -c "
import json,sys,base64
d=json.load(sys.stdin)['return']
o=base64.b64decode(d.get('out-data') or b'').decode().strip()
e=base64.b64decode(d.get('err-data') or b'').decode().strip()
print(o or '(no stdout)')
if e: print('ERR:', e)
sys.exit(0 if d.get('exitcode') in (0, None) else 1)
"
guest_ip=$(virsh qemu-agent-command "$vm" '{"execute":"guest-network-get-interfaces"}' | grep -oE '212\.102\.227\.[0-9]+' | head -1 || true)
if [ "$guest_ip" = "$NEW" ]; then echo "guest=$guest_ip OK"; else echo "guest=$guest_ip MISMATCH"; exit 1; fi
