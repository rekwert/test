# Machine 1 - Front VPS (Traefik + Next.js)

Public-facing portal. Proxies `/api` to Back VPS (Machine 2). HTTPS via Let's Encrypt.

## Specs

| Resource | Minimum |
|----------|---------|
| vCPU | 2 |
| RAM | 8 GB |
| Disk | 60 GB SSD |

## Prerequisites

- Domain pointed to this VPS (A record)
- Machine 2 (Back) deployed — [deploy-machine-2-back.md](./deploy-machine-2-back.md)
- Back UFW allows **8080** from this Front VPS IP

## 1. OS setup

```bash
sudo apt update && sudo apt upgrade -y
sudo apt install -y docker.io docker-compose-v2 ufw curl gettext-base
sudo systemctl enable --now docker
sudo usermod -aG docker $USER
```

## 2. DNS

```
portal.yourdomain.com  A  FRONT_VPS_PUBLIC_IP
```

Wait for propagation before deploy (Let's Encrypt needs valid DNS).

## 3. Clone and configure

```bash
git clone https://github.com/borishru-boop/testVPStrade.git /opt/testVPStrade
cd /opt/testVPStrade
cp infra/docker/.env.front.example infra/docker/.env
nano infra/docker/.env
```

| Variable | Example |
|----------|---------|
| `DOMAIN` | `portal.yourdomain.com` |
| `ACME_EMAIL` | `admin@yourdomain.com` |
| `BACK_GATEWAY_URL` | `http://10.0.0.2:8080` (Back private IP) |
| `NEXT_PUBLIC_API_URL` | `https://portal.yourdomain.com/api/v1` |

## 4. Firewall

```bash
sudo ufw default deny incoming
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

## 5. Deploy

```bash
chmod +x infra/scripts/deploy-front.sh
bash infra/scripts/deploy-front.sh
```

First start may take 1-2 minutes while Traefik obtains SSL certificate.

## 6. Verify

```bash
curl -I https://portal.yourdomain.com
curl https://portal.yourdomain.com/api/v1/../health
# or
curl https://portal.yourdomain.com/../health  # gateway /health via /api path - actually /health is not under /api
```

Gateway health is on Back directly. Through Traefik:

```bash
curl https://portal.yourdomain.com/api/v1/auth/register -X POST \
  -H "Content-Type: application/json" \
  -d '{"email":"test@test.com","password":"Test1234!"}'
```

Open portal in browser: `https://portal.yourdomain.com`

## Architecture

```
Internet
   |
   v
Front VPS :443 (Traefik + TLS)
   |-- /        --> Next.js :3000
   |-- /api/*   --> Back VPS :8080 (gateway)
```

## Web image build note

`NEXT_PUBLIC_API_URL` is embedded at **build time**. For production images:

```bash
docker build --build-arg NEXT_PUBLIC_API_URL=https://portal.yourdomain.com/api/v1 \
  -t ghcr.io/borishru-boop/testvps-trade-web:latest apps/web
```

The web app also falls back to `window.location.origin + /api/v1` in the browser when env is unset.

## Troubleshooting

| Issue | Fix |
|-------|-----|
| ACME failed | Check DNS A record, port 80 open |
| 502 on /api | Check Back VPS running, UFW allows Front IP on 8080 |
| CORS errors | Set `CORS_ORIGINS=https://portal.yourdomain.com` on Back `.env` |

## Next

- Machine 4 (Mail): [mail-poste.md](./mail-poste.md)
- CI/CD for GHCR images: next block
