@echo off
rem Double-click (Windows) to update and start HiringPlug: gets the latest code (if
rem git is installed; otherwise pull in GitHub Desktop first), rebuilds what
rem changed, and opens the app. Your data is kept. Needs Docker Desktop.
cd /d "%~dp0"

docker info >/dev/null 2>&1
if not errorlevel 1 goto docker_ready

rem Docker is not running: find Docker Desktop (installed for all users, for
rem this user only, or wherever the docker command lives) and start it.
set "DD="
for %%p in ("%ProgramFiles%\Docker\Docker\Docker Desktop.exe" "%ProgramW6432%\Docker\Docker\Docker Desktop.exe" "%LOCALAPPDATA%\Programs\DockerDesktop\Docker Desktop.exe" "%LOCALAPPDATA%\Programs\Docker\Docker\Docker Desktop.exe" "%LOCALAPPDATA%\Docker\Docker Desktop.exe") do if not defined DD if exist "%%~p" set "DD=%%~p"
if not defined DD for /f "delims=" %%d in ('where docker 2^>nul') do if not defined DD if exist "%%~dpd..\..\Docker Desktop.exe" set "DD=%%~dpd..\..\Docker Desktop.exe"
if not defined DD goto ask_user
echo Starting Docker Desktop...
start "" "%DD%"
goto wait_docker
:ask_user
echo Docker is not running. Please start Docker Desktop now; this window will wait for it.
:wait_docker
for /l %%i in (1,1,90) do (
  timeout /t 2 /nobreak >nul
  docker info >/dev/null 2>&1 && goto docker_ready
)
echo Problem: Docker is still not running after 3 minutes.
echo Start Docker Desktop, wait until it says "Engine running", then double-click this file again.
pause
exit /b 1
:docker_ready

where git >/dev/null 2>&1
if errorlevel 1 goto build
if not exist .git goto build
echo Getting the latest version...
git pull --ff-only
if errorlevel 1 echo Could not pull (local changes?). Using the code as it is.
:build

echo Building and starting HiringPlug (the first time takes a few minutes)...
docker compose -f deploy/docker-compose.yml up -d --build --remove-orphans
if not errorlevel 1 goto wait_app
echo Problem: Docker could not start HiringPlug (see the messages above).
pause
exit /b 1

:wait_app
echo Waiting for the app...
for /l %%i in (1,1,60) do (
  curl -sf http://localhost:3000 >/dev/null 2>&1 && goto app_ready
  timeout /t 2 /nobreak >nul
)
:app_ready
start "" http://localhost:3000
echo HiringPlug is running at http://localhost:3000 (API docs: http://localhost:8080/docs).
timeout /t 5 >nul
