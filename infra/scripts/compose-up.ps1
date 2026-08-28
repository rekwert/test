# Fix YAML encoding and start stack. Run from repo root:
#   powershell -ExecutionPolicy Bypass -File infra/scripts/compose-up.ps1

$ErrorActionPreference = "Stop"
Set-Location (Resolve-Path (Join-Path $PSScriptRoot "..\.."))
$utf8 = New-Object System.Text.UTF8Encoding $false

Get-ChildItem "infra\docker\*.yml" | ForEach-Object {
    $bytes = [System.IO.File]::ReadAllBytes($_.FullName)
    if ($bytes.Length -ge 2 -and $bytes[1] -eq 0) {
        $text = [System.IO.File]::ReadAllText($_.FullName, [System.Text.Encoding]::Unicode)
        [System.IO.File]::WriteAllText($_.FullName, $text, $utf8)
        Write-Host "Fixed encoding: $($_.Name)"
    }
}

docker compose -f infra/docker/docker-compose.yml config | Out-Null
Write-Host "YAML OK"
docker compose -f infra/docker/docker-compose.yml up -d
docker compose -f infra/docker/docker-compose.yml ps
