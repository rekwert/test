# Deploy workflow — cloud-hustle.com

**Rule: never edit production servers directly. Push to GitHub first, then deploy.**

## Order of operations

1. **Commit and push** to `main` on `github.com/borishru-boop/testVPStrade`
2. **Deploy** on Back (`198.13.189.75`) — images are **built on the server from git**, not pulled from GHCR by default
3. Optional: run GitHub Actions **Docker Build** (`workflow_dispatch`) to refresh GHCR `:latest` for other environments

GHCR `latest` is **manual-only** and often stale. Production Back must use `BUILD_LOCAL=1` (default in `deploy-back.sh`).

Do **not** `scp` hotfix binaries to servers — push to git, then deploy.

## Back (`198.13.189.75`)

```bash
cd /opt/testVPStrade
bash infra/scripts/deploy-back.sh
```

`deploy-back.sh` runs: `git fetch && git reset --hard origin/main` → migrations → **build all Back images** → `up -d` → **gateway route audit**.

To pull prebuilt GHCR images instead (not recommended on prod):

```bash
BUILD_LOCAL=0 bash infra/scripts/deploy-back.sh
```

Audit routes only:

```bash
bash infra/scripts/audit-gateway-routes.sh
```

Build images without full deploy:

```bash
bash infra/scripts/build-back-images.sh
```

Ensure `infra/docker/.env` exists (copy from `.env.back.example`, fill secrets). Key VirtFusion vars:

- `VIRTFUSION_API_URL` / `VIRTFUSION_API_KEY`
- `VIRTFUSION_PLAN_MAP` — catalog plan UUID → VirtFusion package id
- `VIRTFUSION_PACKAGE_ID` / `VIRTFUSION_HYPERVISOR_GROUP_ID`
- `VIRTFUSION_OS_MAP` — optional; worker auto-syncs OS from VirtFusion API
- `BUILD_LOCAL=1` — default; builds all services from git on deploy. Set `BUILD_LOCAL=0` to `docker compose pull` instead.

## Front (`213.148.3.172`)

Separate repo (`FrontVPS`). Use `infra/scripts/deploy-front.sh` after push to that repository.

## DB (`108.174.78.39`)

Schema via `migrate.sh` from back deploy. One-off data fixes use scripts in `infra/scripts/` with `POSTGRES_DSN` env — never commit real passwords.

## VirtFusion NL (`66.248.206.14`)

Fresh install on empty Debian 12: see `VF-REINSTALL-RUNBOOK.md` and official docs https://docs.virtfusion.com/

## Diagnostic scripts

VirtFusion troubleshooting scripts live in `infra/scripts/` (prefix `vf-`). They typically need:

- `NL_PASS` — NL hypervisor root SSH password
- `VIRTFUSION_API_URL` / `VIRTFUSION_API_KEY`
- `POSTGRES_DSN` — portal database (when updating instances)

## What is NOT in git

- `infra/docker/.env` on servers (secrets)
- Live VirtFusion panel state on NL
- Portal DB row-level repairs
