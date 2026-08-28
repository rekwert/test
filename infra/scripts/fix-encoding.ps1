# Fix UTF-16 files on Windows (Docker, Git, Go require UTF-8)
# Run: powershell -ExecutionPolicy Bypass -File infra/scripts/fix-encoding.ps1

$root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$utf8 = New-Object System.Text.UTF8Encoding $false
$skipPattern = '\\(\.git|node_modules|\.next)(\\|$)'

$fixed = 0
$ok = 0

Get-ChildItem -Path $root -Recurse -File -ErrorAction SilentlyContinue |
    Where-Object {
        $_.FullName -notmatch $skipPattern -and
        ($_.Extension -match '^\.(yml|yaml|go|mod|sql|json|js|ts|tsx|md|sh)$' -or
         $_.Name -in @('Dockerfile', 'Makefile', '.env', '.env.example'))
    } |
    ForEach-Object {
        $bytes = [System.IO.File]::ReadAllBytes($_.FullName)
        if ($bytes.Length -ge 2 -and $bytes[1] -eq 0) {
            $text = [System.IO.File]::ReadAllText($_.FullName, [System.Text.Encoding]::Unicode)
            [System.IO.File]::WriteAllText($_.FullName, $text, $utf8)
            Write-Host "Fixed: $($_.FullName.Replace($root, '.'))"
            $script:fixed++
        } else {
            $script:ok++
        }
    }

Write-Host ""
Write-Host "Done. Fixed: $fixed, already UTF-8: $ok"

Set-Location $root
docker compose -f infra/docker/docker-compose.yml config | Out-Null
if ($LASTEXITCODE -eq 0) {
    Write-Host "YAML OK"
}
