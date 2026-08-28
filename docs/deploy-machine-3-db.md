# Machine 3 - Database VPS (PostgreSQL + Redis)

Internal data layer. Firewall: allow 5432/6379 from Back VPS only.

## Specs

| Resource | Minimum |
|----------|---------|
| vCPU | 4 |
| RAM | 32 GB |
| Disk | 300 GB NVMe |

## 1. OS setup (Ubuntu 22.04/24.04)

```bash
sudo apt update && sudo apt upgrade -y
sudo apt install -y docker.io docker-compose-v2 ufw
sudo systemctl enable --now docker
sudo usermod -aG docker $USER
```

## 2. Clone repo

```bash
sudo mkdir -p /opt/testVPStrade
sudo chown $USER:$USER /opt/testVPStrade
git clone https://github.com/borishru-boop/testVPStrade.git /opt/testVPStrade
cd /opt/testVPStrade
```

## 3. Configure

```bash
cp infra/docker/.env.db.example infra/docker/.env
nano infra/docker/.env
```

Set strong passwords and `DB_BIND_IP`.

## 4. Firewall (UFW)

Replace `BACK_VPS_IP` with Machine 2 IP:

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow from BACK_VPS_IP to any port 5432 proto tcp
sudo ufw allow from BACK_VPS_IP to any port 6379 proto tcp
sudo ufw allow OpenSSH
sudo ufw enable
```

## 5. Deploy

```bash
chmod +x infra/scripts/deploy-db.sh infra/scripts/backup-db.sh
bash infra/scripts/deploy-db.sh
```

## 6. Connection strings for Back VPS (Machine 2)

```env
POSTGRES_DSN=postgres://vps:PASSWORD@DB_VPS_IP:5432/vps_platform?sslmode=disable
REDIS_URL=redis://:REDIS_PASSWORD@DB_VPS_IP:6379/0
```

## 7. Backups (cron)

```bash
0 3 * * * /opt/testVPStrade/infra/scripts/backup-db.sh >> /var/log/testVPStrade-backup.log 2>&1
```

Backups: `/var/backups/testVPStrade/` (14 days retention).

## 8. Verify

```bash
cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.db.yml ps
```

## Next

Machine 2 (Back) - next block.
