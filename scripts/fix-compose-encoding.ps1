# Wrapper: fix UTF-16 files then validate compose (Windows)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
node scripts\fix-run.js
if ($LASTEXITCODE -ne 0) { exit 1 }
docker compose -f infra/docker/docker-compose.yml config | Out-Null
Write-Host "compose OK"
