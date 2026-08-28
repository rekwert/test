# Run on template builder VM after Windows + VirtIO + RDP, BEFORE sysprep.
# Ensures shell AppX packages are provisioned for all users on cloned VMs.
$ErrorActionPreference = 'Continue'

$names = @(
    'Microsoft.Windows.Search',
    'Microsoft.Windows.StartMenuExperienceHost',
    'Microsoft.Windows.ShellExperienceHost'
)

foreach ($name in $names) {
    $pkg = Get-AppxProvisionedPackage -Online | Where-Object { $_.DisplayName -eq $name }
    if (-not $pkg) {
        Write-Host "MISSING provisioned: $name"
        continue
    }
    Write-Host "OK provisioned: $name"
}

# Worker applies shell fix over SSH after provision.
Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0 -ErrorAction SilentlyContinue | Out-Null
Set-Service sshd -StartupType Automatic -ErrorAction SilentlyContinue
Start-Service sshd -ErrorAction SilentlyContinue
New-NetFirewallRule -Name 'OpenSSH-Server-In-TCP' -DisplayName 'OpenSSH Server (sshd)' `
    -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22 `
    -ErrorAction SilentlyContinue | Out-Null
Write-Host 'OpenSSH Server enabled (port 22)'

# Install Cloudbase first-boot fix for clones where provision still fails.
& (Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) 'Install-CloudbaseShellFix.ps1')

Write-Host ''
Write-Host 'Verify Search works (Win+S, taskbar search), then sysprep as built-in Administrator.'
