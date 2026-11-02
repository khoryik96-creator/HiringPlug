@echo off
rem Double-click (Windows) to update and start HiringPlug: gets the latest code (if
rem git is installed; otherwise pull in GitHub Desktop first), rebuilds what
rem changed, and opens the app. Your data is kept. Needs Docker Desktop running.
cd /d "%~dp0"

docker info >nul 2>&1
if errorlevel 1 (
  echo Starting Docker Desktop...
  start "" "%ProgramFiles%\Docker\Docker\Docker Desktop.exe"
  for /l %%i in (1,1,60) do (
    timeout /t 2 /nobreak >nul
    docker info >nul 2>&1 && goto docker_ready
  )
  echo Problem: Docker Desktop is not running. Start it and try again.
  pause
  exit /b 1
)
:docker_ready

where git >nul 2>&1
if not errorlevel 1 if exist .git (
  echo Getting the latest version...
  git pull --ff-only || echo Could not pull ^(local changes?^). Using the code as it is.
)

echo Building and starting HiringPlug (the first time takes a few minutes)...
docker compose -f deploy/docker-compose.yml up -d --build --remove-orphans
if errorlevel 1 (
  echo Problem: Docker could not start HiringPlug ^(see above^).
  pause
  exit /b 1
)

echo Waiting for the app...
for /l %%i in (1,1,60) do (
  curl -sf http://localhost:3000 >nul 2>&1 && goto app_ready
  timeout /t 2 /nobreak >nul
)
:app_ready
start "" http://localhost:3000
echo HiringPlug is running at http://localhost:3000 (API docs: http://localhost:8080/docs).
timeout /t 5 >nul
