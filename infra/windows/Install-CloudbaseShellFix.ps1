# Run once on the template builder VM (as Administrator), BEFORE sysprep.
# Installs Fix-WindowsShellApps.ps1 into Cloudbase-Init LocalScripts.
$ErrorActionPreference = 'Stop'

$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$src = Join-Path $here 'Fix-WindowsShellApps.ps1'
if (-not (Test-Path $src)) {
    throw "Missing $src"
}

$destDir = 'C:\Program Files\Cloudbase Solutions\Cloudbase-Init\LocalScripts'
$dest = Join-Path $destDir 'Fix-WindowsShellApps.ps1'
if (-not (Test-Path $destDir)) {
    New-Item -ItemType Directory -Path $destDir -Force | Out-Null
}
Copy-Item -Path $src -Destination $dest -Force
Write-Host "Installed LocalScript: $dest"

$conf = 'C:\Program Files\Cloudbase Solutions\Cloudbase-Init\conf\cloudbase-init.conf'
if (-not (Test-Path $conf)) {
    Write-Warning "cloudbase-init.conf not found at $conf — enable localscripts plugin manually"
    exit 0
}

$text = Get-Content $conf -Raw
if ($text -notmatch '(?m)^plugins\s*=') {
    Write-Warning "No plugins= line in $conf — add localscripts to plugins manually"
    exit 0
}

if ($text -match 'localscripts') {
    Write-Host 'localscripts plugin already enabled'
    exit 0
}

$text = $text -replace '(?m)^(plugins\s*=.*)$', '$1,localscripts'
Set-Content -Path $conf -Value $text -Encoding UTF8
Write-Host 'Added localscripts to plugins in cloudbase-init.conf'
