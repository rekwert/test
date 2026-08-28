# Poste.io mail server (Machine 4)

Separate VPS for email. Platform sends mail via SMTP from `notification` on Machine 2 (Back).

## Architecture

```
Machine 2 (Back)                    Machine 4 (Mail)
  notification-service  --SMTP:587-->  Poste.io
  auth-service          --HTTP-->     (internal only)
```

## Poste.io install (Docker Compose)

```bash
cp infra/docker/.env.mail.example infra/docker/.env
bash infra/scripts/deploy-mail.sh
```

Or see full guide: [deploy-machine-4-mail.md](./deploy-machine-4-mail.md)

Official image docs: https://poste.io/doc/getting-started

Open admin: `https://mail.yourdomain.com/admin`

## Poste.io setup checklist

1. Set hostname: `mail.yourdomain.com`
2. Create domain: `yourdomain.com`
3. Create mailbox: `noreply@yourdomain.com` (password for SMTP)
4. Optional: `support@yourdomain.com`
5. Enable DKIM in Poste admin, copy DNS records
6. Restrict admin panel (firewall: allow your IP only)

## DNS records

| Type | Name | Value |
|------|------|-------|
| A | mail | IP of Mail VPS |
| MX | @ | mail.yourdomain.com (priority 10) |
| SPF | @ | `v=spf1 ip4:MAIL_VPS_IP -all` |
| DKIM | (from Poste admin) | TXT record |
| DMARC | _dmarc | `v=DMARC1; p=none; rua=mailto:admin@yourdomain.com` |
| PTR | (at hoster) | mail.yourdomain.com |

## Back VPS (Machine 2) — notification env

Copy `infra/mail/poste.env.example` values into `.env` on Back server:

```env
SMTP_HOST=mail.yourdomain.com
SMTP_PORT=587
SMTP_TLS=true
SMTP_USER=noreply@yourdomain.com
SMTP_PASS=***
SMTP_FROM=noreply@yourdomain.com
```

Restart notification:

```bash
docker compose -f infra/docker/docker-compose.back.yml up -d notification
```

## Test sending

From Back VPS (after Poste is up):

```bash
curl -X POST http://localhost:8004/send \
  -H "Content-Type: application/json" \
  -d '{"to":"you@gmail.com","template":"email_verify","locale":"ru","data":{"code":"123456"}}'
```

Or full flow: register at Portal, check inbox.

## Local dev (no Poste)

Uses Mailpit automatically:

- SMTP: `mailpit:1025`, no TLS, no auth
- UI: http://localhost:8025

## Email templates (RU + EN)

| Template | Used for |
|----------|----------|
| email_verify | Registration 6-digit code |
| password_reset | Forgot password 6-digit code |

## Firewall Mail VPS

| Port | Access |
|------|--------|
| 587 | Back VPS IP only (submission) |
| 443 | Admin (your IP only) |
| 25 | Optional, for inbound mail |

## When ready

1. Buy domain + Mail VPS
2. Install Poste.io
3. Configure DNS + PTR
4. Update Back `.env` SMTP vars
5. `docker compose up -d notification auth` — done
