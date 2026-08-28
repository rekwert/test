# Machine 4 - Mail VPS (Poste.io)

Self-hosted SMTP for verification codes and notifications. Only Back VPS sends mail via port 587.

## Specs

| Resource | Minimum |
|----------|---------|
| vCPU | 2 |
| RAM | 4 GB |
| Disk | 40 GB SSD |

## Prerequisites

- Domain (same as portal or dedicated mail domain)
- Machine 2 (Back) deployed
- PTR record from hoster (critical for deliverability)

## 1. OS setup

```bash
sudo apt update && sudo apt upgrade -y
sudo apt install -y docker.io docker-compose-v2 ufw curl
sudo systemctl enable --now docker
sudo usermod -aG docker $USER
```

## 2. DNS (before or right after deploy)

| Type | Name | Value |
|------|------|-------|
| A | mail | Mail VPS public IP |
| MX | @ | mail.yourdomain.com (10) |
| SPF | @ | `v=spf1 ip4:MAIL_IP -all` |
| DKIM | (Poste admin) | TXT |
| DMARC | _dmarc | `v=DMARC1; p=none; rua=mailto:admin@yourdomain.com` |
| PTR | (hoster) | mail.yourdomain.com |

## 3. Clone and configure

```bash
git clone https://github.com/borishru-boop/testVPStrade.git /opt/testVPStrade
cd /opt/testVPStrade
cp infra/docker/.env.mail.example infra/docker/.env
nano infra/docker/.env
```

| Variable | Example |
|----------|---------|
| `POSTE_HOSTNAME` | `mail.yourdomain.com` |
| `BACK_VPS_IP` | Back VPS IP (for UFW) |
| `ADMIN_IP` | Your home IP (admin UI) |
| `SMTP_USER` | `noreply@yourdomain.com` |

## 4. Firewall

```bash
sudo ufw default deny incoming
sudo ufw allow OpenSSH
sudo ufw allow from BACK_VPS_IP to any port 587 proto tcp
sudo ufw allow from ADMIN_IP to any port 443 proto tcp
sudo ufw allow 25/tcp
sudo ufw enable
```

Port **587** — only Back VPS. Admin **443** — only your IP.

## 5. Deploy

```bash
chmod +x infra/scripts/deploy-mail.sh
bash infra/scripts/deploy-mail.sh
```

Open `https://mail.yourdomain.com/admin` and complete first-time setup.

## 6. Poste.io setup

1. Hostname: `mail.yourdomain.com`
2. Add domain: `yourdomain.com`
3. Create mailbox: `noreply@yourdomain.com` — save password
4. Mail server → Enable DKIM → add TXT to DNS
5. Test send from Poste admin (optional)

## 7. Connect Back VPS

On **Machine 2**, edit `infra/docker/.env`:

```env
SMTP_HOST=mail.yourdomain.com
SMTP_PORT=587
SMTP_TLS=true
SMTP_USER=noreply@yourdomain.com
SMTP_PASS=mailbox-password-from-poste
SMTP_FROM=noreply@yourdomain.com
```

Restart notification:

```bash
cd /opt/testVPStrade/infra/docker
docker compose -f docker-compose.back.yml up -d notification
```

## 8. Test

From Back VPS:

```bash
curl -X POST http://127.0.0.1:8004/send \
  -H "Content-Type: application/json" \
  -d '{"to":"you@gmail.com","template":"email_verify","locale":"ru","data":{"code":"123456"}}'
```

Or register at portal — check real inbox.

## Architecture

```
Back VPS (notification)  --SMTP:587/TLS-->  Mail VPS (Poste.io)
                                                    |
                                              Internet (outbound mail)
```

Auth service does **not** talk to Poste directly — only notification via SMTP.

## Local dev

No Poste needed locally — Mailpit at http://localhost:8025

## More

- DNS / templates detail: [mail-poste.md](./mail-poste.md)
- Back SMTP env sample: [infra/mail/poste.env.example](../infra/mail/poste.env.example)

## Next

- CI/CD (GHCR images): next block
- Machine 5+ VirtFusion: when hardware ready
