# VF → OpenStack migration checklist (без Ceph)

Локальная копия для разработки: `testVPStrade-openstack-dev`. Оригинал не трогаем до успешного E2E.

## Соответствие полей

| VF / portal | OpenStack | Примечание |
|-------------|-----------|------------|
| `instances.external_id` (numeric VF server id) | Nova `server UUID` | Новые заказы получают UUID сразу. Старые VF — отдельная миграция (см. ниже) |
| `nodes.external_id` (VF hypervisor group id) | Nova hypervisor UUID | `openstack hypervisor list` → ID в колонке Hypervisor ID |
| `nodes.external_id` (hostname) | То же имя | Можно хранить hostname, если совпадает с `HypervisorHostname` |
| Catalog plan UUID | Nova flavor | `OPENSTACK_PLAN_MAP=<plan-uuid>:<flavor>` |
| `os_template_id` | Glance image | `OPENSTACK_OS_MAP=ubuntu-22.04:<image>` |
| Primary IPv4 | Neutron Floating IP | `OPENSTACK_FLOATING_NETWORK_ID` обязателен для публичных IP |
| Extra IPv4 | Доп. NIC + FIP | Одна FIP на интерфейс |
| Snapshots | Nova `createImage` → Glance | |
| Console | noVNC через Nova console | |
| Metrics | Nova `/diagnostics` | CPU/RAM/сеть, если включено на cloud |
| SMTP block | SSH на compute (`HV_SSH_*`) | Опционально; без SSH — feature off |

## Настройка нод (6 compute)

1. На control plane: `openstack hypervisor list` — записать UUID и hostname каждой ноды.
2. В БД `vps.nodes` для каждого региона обновить `external_id` на **Nova hypervisor UUID** (не VF group id).
3. Env для pinning (если UUID в БД, а schedule по hostname):

```env
OPENSTACK_HV_MAP=<hypervisor-uuid>:compute-nl-01,<uuid2>:compute-nl-02
OPENSTACK_NODE_MAP=<portal-node-uuid>:compute-nl-01
```

При создании VM Nova получает scheduler hint `host=<hostname>`.

## Env (минимум для prod-like dev)

```env
HYPERVISOR_PROVIDER=openstack
OPENSTACK_AUTH_URL=https://keystone:5000/v3
OPENSTACK_REGION=RegionOne
OPENSTACK_USERNAME=...
OPENSTACK_PASSWORD=...
OPENSTACK_PROJECT_NAME=...
OPENSTACK_DOMAIN_NAME=Default
OPENSTACK_NETWORK_ID=<tenant-network>
OPENSTACK_FLOATING_NETWORK_ID=<external-network>
OPENSTACK_PLAN_MAP=11111111-1111-1111-1111-111111111101:m1.small
OPENSTACK_OS_MAP=ubuntu-22.04:Ubuntu-22.04,debian-12:Debian-12
OPENSTACK_PROVISION_REGIONS=nl,fi,de,gb
```

## Provision flow (OpenStack vs VF)

| Шаг | VF | OpenStack |
|-----|-----|-----------|
| Allocate | Пустая VM на hypervisor | Nova boot from Glance (сразу с OS) |
| Build | Установка OS + IPv4 | Wait ACTIVE + assign Floating IP |
| Guest setup | SSH, password, qemu-ga | То же (worker) |

`ServerNeedsBuild`: для OpenStack не пересобирает OS на ACTIVE; повторный вызов только если нет публичного IP (FIP).

## Миграция существующих VF-инстансов

Автоматического переноса **нет** — только новые provision на OpenStack.

Варианты для cutover без потери данных:

1. **Maintenance window**: остановить VF VPS → snapshot/export → import в Glance/Cinder → создать Nova server с тем же `external_id` в portal (ручной SQL + скрипт).
2. **Parallel run**: новые заказы на OpenStack; старые VF до end-of-life с `provider='virtfusion'` (в dev-копии VF удалён — только для prod cutover plan).

Перед cutover проверить по чеклисту:

- [ ] Все catalog plans в `OPENSTACK_PLAN_MAP`
- [ ] Все OS templates в `OPENSTACK_OS_MAP`
- [ ] Все nodes `external_id` = Nova hypervisor UUID
- [ ] Floating IP pool достаточен (400+ VPS)
- [ ] E2E: order → running → console → reinstall → change IP → resize → snapshot → destroy
- [ ] Node sync: admin видит utilization и status `online`

## Инфра (без Ceph)

- 1× control plane (Nova, Neutron, Keystone, Glance)
- 6× compute с local NVMe (Cinder backend = LVM/local или boot-from-volume на локальном диске)
- Floating network + tenant network
- Образы в Glance (sync с текущими VF templates)

## Тест локально

Mock: `OPENSTACK_USE_MOCK=true` — полный portal flow без cloud.

Real: см. `OPENSTACK-DEV.md`, override `OPENSTACK_USE_MOCK=false` в compose override.
