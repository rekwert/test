# Rebuild auth + notification + web after email changes
# Run: powershell -ExecutionPolicy Bypass -File scripts/rebuild-email.ps1

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

node scripts\fix-run.js
if ($LASTEXITCODE -ne 0) { exit 1 }

docker compose -f infra/docker/docker-compose.yml build auth notification web gateway
if ($LASTEXITCODE -ne 0) { exit 1 }

docker compose -f infra/docker/docker-compose.yml up -d auth notification web gateway
Write-Host "Done. Mailpit: http://localhost:8025 | Portal: http://localhost:3000"
