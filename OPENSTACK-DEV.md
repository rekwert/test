# OpenStack dev sandbox (local only)

Это **локальная копия** `testVPStrade` для разработки OpenStack-адаптера.
**Не пушить в git.** Оригинал: `../testVPStrade`.

VirtFusion **полностью удалён** из этой копии. Единственный hypervisor data plane: **OpenStack** (или mock).

## Быстрый старт (mock)

**Полная инструкция:** [LOCAL-DEV.md](./LOCAL-DEV.md) — portal + billing + mock/real OpenStack на Windows.

```powershell
cd "d:\saas решение\testVPStrade-openstack-dev"
.\infra\scripts\local-dev-up.ps1
```

Portal: http://localhost:3000 | API: http://localhost:8080 | Mailpit: http://localhost:8025

## OpenStack (MicroStack / dev cloud)

```env
HYPERVISOR_PROVIDER=openstack
OPENSTACK_AUTH_URL=http://10.0.0.10:5000/v3
OPENSTACK_REGION=RegionOne
OPENSTACK_USERNAME=admin
OPENSTACK_PASSWORD=secret
OPENSTACK_PROJECT_NAME=admin
OPENSTACK_DOMAIN_NAME=Default
OPENSTACK_NETWORK_ID=<neutron-network-uuid>
OPENSTACK_FLOATING_NETWORK_ID=<external-network-uuid>
OPENSTACK_PLAN_MAP=11111111-1111-1111-1111-111111111101:m1.small
OPENSTACK_OS_MAP=ubuntu-22.04:Ubuntu-22.04
OPENSTACK_PROVISION_REGIONS=nl,fi,de
```

## Реализовано / parity с VF

| Метод | Статус |
|-------|--------|
| Allocate / Build / Create / Get | ✅ Nova (+ Build = wait ACTIVE + FIP) |
| Node pinning | ✅ OPENSTACK_NODE_MAP / OPENSTACK_HV_MAP / hostname in external_id |
| Start / Stop / Reboot / Delete | ✅ (+ FIP cleanup on delete) |
| Floating IP / Change IP / Extra IPv4 | ✅ Neutron |
| GetConsole (noVNC) | ✅ |
| Reinstall (Rebuild) | ✅ |
| ResetRootPassword / SetRootPassword | ✅ |
| cloud-init SSH + password | ✅ UserData at create |
| Resize / SyncServerPlan | ✅ |
| Snapshots | ✅ |
| Metrics | ✅ Nova diagnostics |
| Node sync | ✅ hypervisor list + commissioned=3 |

Полный чеклист миграции: [OPENSTACK-MIGRATION.md](./OPENSTACK-MIGRATION.md)

## Структура

```
services/vps/internal/openstack/
  config.go, client.go, adapter.go
  console.go, floatingip.go, ip.go, network.go
  password.go, resize.go, snapshots.go, tls.go
```
