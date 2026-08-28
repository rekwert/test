#Requires -Version 5.1
$ErrorActionPreference = "Stop"
$Root = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$DockerDir = Join-Path $Root "infra\docker"
Push-Location $DockerDir
try {
    docker compose --env-file .env.local down
} finally {
    Pop-Location
}
