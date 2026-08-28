# Machine 2 - Back VPS (API + workers)

Gateway, auth, billing, vps, notification and workers. Connects to Machine 3 (DB) by IP.

## Specs

| Resource | Minimum |
|----------|---------|
| vCPU | 4-6 |
| RAM | 16 GB |
| Disk | 100 GB SSD |

## Prerequisites

- Machine 3 (DB) deployed — [deploy-machine-3-db.md](./deploy-machine-3-db.md)
- Docker images in GHCR (or build locally until CI is ready)
- SMTP credentials from Machine 4 (Poste.io) when mail VPS is up

## 1. OS setup

Same as DB machine: Ubuntu, Docker, UFW.

```bash
sudo apt update && sudo apt upgrade -y
sudo apt install -y docker.io docker-compose-v2 ufw curl
sudo systemctl enable --now docker
sudo usermod -aG docker $USER
```

## 2. Clone repo

```bash
git clone https://github.com/borishru-boop/testVPStrade.git /opt/testVPStrade
cd /opt/testVPStrade
```

## 3. Configure

```bash
cp infra/docker/.env.back.example infra/docker/.env
nano infra/docker/.env
```

| Variable | Description |
|----------|-------------|
| `DB_VPS_IP` | IP of Machine 3 |
| `POSTGRES_DSN` | Full DSN with real password and IP |
| `REDIS_URL` | `redis://:PASSWORD@DB_IP:6379/0` |
| `JWT_SECRET` | Long random string |
| `CORS_ORIGINS` | `https://your-domain.com` |
| `SMTP_*` | Poste.io mailbox (Machine 4) |
| `VIRTFUSION_*` | Leave mock/stub until Machine 5 |

## 4. Firewall

Replace `FRONT_VPS_IP` and `DB_VPS_IP`:

```bash
sudo ufw default deny incoming
sudo ufw allow OpenSSH
sudo ufw allow from FRONT_VPS_IP to any port 8080 proto tcp
sudo ufw allow out to DB_VPS_IP port 5432 proto tcp
sudo ufw allow out to DB_VPS_IP port 6379 proto tcp
sudo ufw enable
```

Port **8080** is the API gateway — only Front VPS should reach it.

## 5. Deploy

```bash
chmod +x infra/scripts/migrate.sh infra/scripts/deploy-back.sh
bash infra/scripts/deploy-back.sh
```

This runs migrations, pulls images, starts all containers.

## 6. Verify

```bash
curl http://127.0.0.1:8080/health
curl -X POST http://127.0.0.1:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@test.com","password":"Test1234!","locale":"ru"}'
```

From Front VPS (after Block 3):

```bash
curl http://BACK_IP:8080/health
```

## 7. Services

| Service | Internal port | Public |
|---------|---------------|--------|
| gateway | 8080 | Front VPS only |
| auth | 8001 | internal |
| billing | 8002 | internal |
| vps | 8003 | internal |
| notification | 8004 | internal |

## 8. Update / rollback

```bash
cd /opt/testVPStrade
git pull
# set IMAGE_TAG in .env if using versioned images
bash infra/scripts/deploy-back.sh
```

## Next

- Machine 1 (Front): Traefik + Next.js, proxy `/api` to Back — next block
- Machine 4 (Mail): [mail-poste.md](./mail-poste.md)
