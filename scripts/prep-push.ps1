$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot\..

Write-Host '=== testVPStrade: prep push ==='
if (Test-Path .env) { Write-Host '.env exists locally (gitignored)' }
Write-Host '1/3 UTF-8 fix...'
node scripts\fix-run.js
Write-Host '2/3 Go build check...'
cmd /c scripts\verify-build.cmd
if ($LASTEXITCODE -ne 0) { throw 'verify-build failed' }
Write-Host '3/3 git status...'
git status -sb
git diff --stat
