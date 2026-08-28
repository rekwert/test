#ps1_sysnative
# Re-register shell UWP apps broken after sysprep /generalize (Search, Start menu).
# Used by Cloudbase-Init LocalScripts and VPS worker post-provision SSH fix.
$ErrorActionPreference = 'Continue'

$stamp = 'C:\ProgramData\VPSTrade\shell-apps-fixed.stamp'
if ($env:VPS_FORCE_SHELL_FIX -ne '1' -and (Test-Path $stamp)) {
    exit 0
}

$adminSid = 'S-1-5-32-544'
$systemApps = @(
    'Microsoft.Windows.Search_cw5n1h2txyewy',
    'ShellExperienceHost_cw5n1h2txyewy',
    'Microsoft.Windows.StartMenuExperienceHost_cw5n1h2txyewy'
)

foreach ($app in $systemApps) {
    $dir = Join-Path $env:Windir "SystemApps\$app"
    if (-not (Test-Path $dir)) { continue }

    & takeown /f $dir /r /d y 2>$null | Out-Null
    & icacls $dir /grant "${adminSid}:(OI)(CI)F" /t 2>$null | Out-Null
    & icacls $dir /grant 'SYSTEM:(OI)(CI)F' /t 2>$null | Out-Null

    $manifest = Join-Path $dir 'AppxManifest.xml'
    if (Test-Path $manifest) {
        Add-AppxPackage -Register -DisableDevelopmentMode -Path $manifest -ErrorAction SilentlyContinue | Out-Null
    }
}

$displayNames = @(
    'Microsoft.Windows.Search',
    'Microsoft.Windows.StartMenuExperienceHost',
    'Microsoft.Windows.ShellExperienceHost'
)

foreach ($name in $displayNames) {
    Get-AppxPackage -AllUsers $name -ErrorAction SilentlyContinue | ForEach-Object {
        $manifest = Join-Path $_.InstallLocation 'AppxManifest.xml'
        if (Test-Path $manifest) {
            Add-AppxPackage -Register -DisableDevelopmentMode -Path $manifest -ErrorAction SilentlyContinue | Out-Null
        }
    }
}

Set-Service WSearch -StartupType Automatic -ErrorAction SilentlyContinue
Start-Service WSearch -ErrorAction SilentlyContinue

Stop-Process -Name SearchHost, SearchUI, StartMenuExperienceHost, ShellExperienceHost -Force -ErrorAction SilentlyContinue
Stop-Process -Name explorer -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2
Start-Process explorer

$stampDir = Split-Path $stamp -Parent
if (-not (Test-Path $stampDir)) {
    New-Item -ItemType Directory -Path $stampDir -Force | Out-Null
}
New-Item -ItemType File -Path $stamp -Force | Out-Null
exit 0
