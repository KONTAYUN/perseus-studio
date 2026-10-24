@echo off
setlocal
cd /d "%~dp0"
title Perseus Studio - local start

rem Host port. Default 18080 (8080 is often taken). Override with PERSEUS_PORT=xxxx in .env
set PERSEUS_PORT=18080
if exist ".env" (
  for /f "usebackq tokens=1,* delims==" %%a in (".env") do (
    if /i "%%a"=="PERSEUS_PORT" set PERSEUS_PORT=%%b
  )
)

echo [1/4] Checking Docker...
docker info >nul 2>&1
if errorlevel 1 (
  echo.
  echo Docker is not running. Start Docker Desktop, wait until it shows "Engine running",
  echo then run this script again.
  echo.
  pause
  exit /b 1
)

if not exist ".env" (
  copy /y ".env.example" ".env" >nul
  echo Created .env from .env.example
)

echo [2/4] Building and starting the container (first build can take several minutes)...
docker compose up -d --build
if errorlevel 1 (
  echo.
  echo Build or start failed. If the error is about pulling base images from docker.io,
  echo set a mirror in .env, for example:
  echo     IMAGE_REGISTRY=docker.m.daocloud.io
  echo If it says the port is already allocated, add PERSEUS_PORT=28080 to .env.
  echo Then run this script again. Otherwise copy the error text above and send it to Claude.
  echo.
  pause
  exit /b 1
)

echo [3/4] Waiting for Perseus on http://localhost:%PERSEUS_PORT% ...
set /a tries=0
:wait
curl.exe -s -f http://127.0.0.1:%PERSEUS_PORT%/api/ready 2>nul | findstr /c:"scheduler" >nul
if not errorlevel 1 goto ready
set /a tries+=1
if %tries% geq 60 (
  echo.
  echo Perseus did not become ready within 2 minutes. Recent logs:
  docker compose logs --tail 40 webide
  echo.
  pause
  exit /b 1
)
timeout /t 2 /nobreak >nul
goto wait

:ready
echo [4/4] Ready. Opening http://localhost:%PERSEUS_PORT%
start "" http://localhost:%PERSEUS_PORT%
echo.
echo The first registered user becomes the admin.
echo View logs : docker compose logs -f webide
echo Stop      : docker compose stop
echo.
pause
