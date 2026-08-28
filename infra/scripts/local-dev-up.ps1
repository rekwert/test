#Requires -Version 5.1
$ErrorActionPreference = "Stop"

$Root = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$DockerDir = Join-Path $Root "infra\docker"
$EnvFile = Join-Path $DockerDir ".env.local"
$Example = Join-Path $DockerDir ".env.local.example"

if (-not (Test-Path $EnvFile)) {
    Copy-Item $Example $EnvFile
    Write-Host "Created $EnvFile from example"
}

function Test-DockerReady {
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    & docker info *> $null
    $ok = ($LASTEXITCODE -eq 0)
    $ErrorActionPreference = $prev
    return $ok
}

function Ensure-Docker {
    if (Test-DockerReady) { return }
    Write-Host "Docker is not running. Starting Docker Desktop..."
    $desktop = "${env:ProgramFiles}\Docker\Docker\Docker Desktop.exe"
    if (Test-Path $desktop) {
        Start-Process $desktop | Out-Null
    } else {
        throw "Docker Desktop not found. Install Docker Desktop and retry."
    }
    $deadline = (Get-Date).AddMinutes(5)
    while ((Get-Date) -lt $deadline) {
        Start-Sleep -Seconds 8
        if (Test-DockerReady) {
            Write-Host "Docker is ready."
            return
        }
        Write-Host "Waiting for Docker..."
    }
    throw "Docker did not start within 5 minutes. Open Docker Desktop manually."
}

Ensure-Docker

Push-Location $DockerDir
try {
    Write-Host "Building and starting local stack (mock OpenStack by default)..."
    $env:COMPOSE_PARALLEL_LIMIT = "2"
    $env:DOCKER_BUILDKIT = "1"
    docker compose --env-file .env.local up -d --build
    if ($LASTEXITCODE -ne 0) {
        Write-Host "First compose attempt failed (often Docker OOM/disk). Retrying without parallel service build..."
        docker compose --env-file .env.local build --parallel 2 gateway auth billing vps notification support 2>$null
        docker compose --env-file .env.local build web 2>$null
        docker compose --env-file .env.local up -d
        if ($LASTEXITCODE -ne 0) { throw "docker compose failed after retry" }
    }

    Write-Host ""
    Write-Host "=== Local dev URLs ==="
    Write-Host "Portal:    http://localhost:3000"
    Write-Host "API:       http://localhost:8080/api/v1"
    Write-Host "Health:    http://localhost:8080/health"
    Write-Host "Mailpit:   http://localhost:8025"
    Write-Host ""
    Write-Host "Register: POST http://localhost:8080/api/v1/auth/register"
    Write-Host '  body: {"email":"dev@test.local","password":"Test1234!","locale":"ru"}'
    Write-Host ""
    Write-Host "Logs: docker compose --env-file .env.local logs -f vps-worker"
    Write-Host "Stop: .\infra\scripts\local-dev-down.ps1"
} finally {
    Pop-Location
}
