# Rebuild auth + gateway + web
# Run: powershell -ExecutionPolicy Bypass -File scripts/rebuild-auth.ps1

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

node scripts\fix-run.js
if ($LASTEXITCODE -ne 0) { exit 1 }

docker compose -f infra/docker/docker-compose.yml build auth web gateway
if ($LASTEXITCODE -ne 0) { exit 1 }

docker compose -f infra/docker/docker-compose.yml up -d auth web gateway
Write-Host "Done. Test: http://localhost:8080/health"
