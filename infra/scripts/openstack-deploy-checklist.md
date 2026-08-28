# Deploy checklist — OpenStack NL (перед первым тестом)

## ⚠️ Критично: куда деплоить код

| Сервер | Что заливать | Когда |
|--------|--------------|-------|
| **Control BM** (Ubuntu 22.04) | `infra/openstack/control-install.sh`, `bootstrap-dev.sh` | Сейчас, пока ставится ОС |
| **NL back VPS** (portal/worker) | **Новый vps image БЕЗ VirtFusion** | **Только после** OpenStack готов + pilot compute **и** drain VF на pilot-ноде |

Пока на 5 BM живут **VF-клиенты**, деплой `testVPStrade-openstack-dev` на back **сломает** управление существующими VPS (VF adapter удалён).

---

## 1. Control BM (OpenStack)

```bash
# на control после Ubuntu 22.04
git clone <repo> /opt/openstack-dev   # или scp infra/openstack/
cd /opt/openstack-dev/infra/openstack
chmod +x control-install.sh bootstrap-dev.sh
sudo ./control-install.sh
source /root/demo-openrc
openstack hypervisor list
```

Firewall: NL back private IP → control `5000,8774,9696,9292`.

---

## 2. NL back VPS — env (пока можно подготовить .env, не перезапускать worker)

Добавить в `infra/docker/.env` (из `/root/openstack-portal-dev.env` на control):

```env
OPENSTACK_USE_MOCK=false
OPENSTACK_AUTH_URL=https://<control-private-ip>:5000/v3
OPENSTACK_USERNAME=admin
OPENSTACK_PASSWORD=...
OPENSTACK_PROJECT_NAME=admin
OPENSTACK_DOMAIN_NAME=Default
OPENSTACK_INSECURE_TLS=true
OPENSTACK_NETWORK_ID=<uuid>
OPENSTACK_FLOATING_NETWORK_ID=<uuid>
OPENSTACK_PLAN_MAP=11111111-1111-1111-1111-111111111101:m1.small,...
OPENSTACK_OS_MAP=ubuntu-22.04:ubuntu-22.04,debian-12:debian-12
OPENSTACK_PROVISION_REGIONS=nl
OPENSTACK_HV_MAP=<hypervisor-uuid>:compute-hostname,...
```

---

## 3. Тесты кода (локально перед push)

```powershell
$env:GOMODCACHE="D:\saas-cache\gomodcache"
$env:GOCACHE="D:\saas-cache\go-build"
$env:GOTMPDIR="D:\saas-cache\go-tmp"
cd services\vps
go build -o D:\saas-cache\bin\vps-worker.exe ./cmd/worker
go test ./internal/openstack/... ./internal/hypervisor/... ./internal/store/... ./cmd/worker/...
```

---

## 4. Preflight с back VPS

```bash
cd /opt/testVPStrade
set -a; source infra/docker/.env; set +a
bash infra/scripts/openstack-preflight.sh
```

---

## 5. Pilot provision (одна compute после join)

1. `vps.nodes.external_id` = Nova hypervisor UUID pilot-ноды  
2. `BUILD_LOCAL=1 bash infra/scripts/deploy-back.sh` — **только когда VF drained на этой ноде**  
3. Заказ PROSTO-1 nl + ubuntu-22.04  
4. `docker logs -f docker-vps-worker-1`  
5. `openstack server list`  
6. Portal: running + IP + console  

---

## 6. Go/no-go

| Проверка | |
|----------|--|
| `openstack token issue` | ✅ |
| `openstack hypervisor list` ≥ 1 | ✅ |
| `OPENSTACK_PLAN_MAP` / `OPENSTACK_OS_MAP` | ✅ |
| Floating network + free IP | ✅ |
| Worker log: `openstack: created server` | ✅ |
| `instances.external_id` = Nova UUID | ✅ |
