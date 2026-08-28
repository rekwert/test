# VirtFusion — fresh install on empty NL (66.248.206.14)

Official docs: https://docs.virtfusion.com/  
Hypervisor: https://docs.virtfusion.com/installation/hypervisor/

## Server

- **NL node:** `66.248.206.14` (Hostkey, Debian 12 preferred)
- **Back:** `198.13.189.75` — worker reads `VIRTFUSION_*` from `/opt/testVPStrade/infra/docker/.env`

## 0. Network BEFORE VirtFusion

Hostkey-style bridge (adjust NIC name if needed):

```
br0 = 66.248.206.14/24, gw 66.248.206.1, port enp2s0f0 (or current primary NIC)
VM IPs: 66.248.206.21, 66.248.206.40, 66.248.206.61
```

Without `br0`, commission/network steps can drop the host off the internet.

## 1. Install (empty host)

From SSH as root on NL (or via back jump):

```bash
# Hypervisor
( set -euo pipefail
  apt-get update && apt-get install -y curl
  curl -fsSL https://install.virtfusion.net/hypervisor-install.sh | bash
)

# Control server (same host — single-node, max 99 VMs)
curl -fsSL https://install.virtfusion.net/install-control-debian-12.sh | sh -s -- --verbose
```

Or use the local helper (also wipes old VF if present):

```bash
bash /root/vf-full-reinstall.sh
# from back: scp infra/scripts/vf-full-reinstall.sh root@66.248.206.14:/root/
```

Installer prints:

- URL: `https://66.248.206.14/login`
- Username: `admin@admin.com` (or printed email)
- Password: (random, in install log)

## 2. Panel setup (~15 min)

1. **Login** → activate paid license (or evaluation for test)
2. **Compute → Hypervisors → Add** → IP `66.248.206.14` → complete commission wizard
3. **Network tab:** bridge `br0`, correct physical interface
4. **Storage tab:** path `/home/vf-data/disk`, Test Connection → Update
5. **Connectivity → IP Blocks:** Gateway `66.248.206.1/24`, pool `.21` / `.40` / `.61`
6. **Server → Packages:** create packages matching portal plans — note package IDs
7. **Media → OS Templates:** download Ubuntu/Debian/Alma/etc.
8. **Configuration → API:** token name `cloud-hustle`, allow `198.13.189.75` or `*`

## 3. Update back worker

On `198.13.189.75`:

```bash
nano /opt/testVPStrade/infra/docker/.env
# VIRTFUSION_API_URL=https://66.248.206.14/api/v1
# VIRTFUSION_API_KEY=<new token>
# VIRTFUSION_PACKAGE_ID=1
# VIRTFUSION_HYPERVISOR_GROUP_ID=1
# VIRTFUSION_PLAN_MAP=...
# VIRTFUSION_USE_MOCK=false
docker compose -f /opt/testVPStrade/infra/docker/docker-compose.back.yml up -d vps vps-worker
```

## 4. Verify

```bash
curl -sk -X POST "https://66.248.206.14/api/v1/servers" \
  -H "Authorization: Bearer $VIRTFUSION_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"packageId":1,"userId":1,"hypervisorId":1}'
```

Should return a server id (not license/`EC 8` errors). Then create a portal order and confirm IP + credentials.
