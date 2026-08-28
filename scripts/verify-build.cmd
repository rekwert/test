@echo off
cd /d "%~dp0.."
echo === fix-run ===
node scripts\fix-run.js
if errorlevel 1 exit /b 1

echo === go build gateway ===
cd apps\gateway
go mod tidy
go build -o nul ./...
if errorlevel 1 exit /b 1
cd ..\..

echo === go build vps ===
cd services\vps
go mod tidy
go build -o nul ./...
if errorlevel 1 exit /b 1
cd ..\..

echo === go build billing ===
cd services\billing
go mod tidy
go build -o nul ./...
if errorlevel 1 exit /b 1
cd ..\..

echo === go build auth ===
cd services\auth
go build -o nul ./...
if errorlevel 1 exit /b 1
cd ..\..

echo === go build notification ===
cd services\notification
go build -o nul ./...
if errorlevel 1 exit /b 1
cd ..\..

echo All builds OK
