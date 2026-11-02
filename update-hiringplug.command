#!/bin/bash
# Double-click (macOS) to update and start HiringPlug: gets the latest code (if
# git is installed; otherwise pull in GitHub Desktop first), rebuilds what
# changed, and opens the app. Your data is kept. Needs Docker Desktop running.
cd "$(dirname "$0")" || exit 1

fail() { echo; echo "Problem: $1"; read -r -p "Press Enter to close." _; exit 1; }

if ! docker ps >/dev/null 2>&1; then
  open -a Docker 2>/dev/null
  echo "Starting Docker Desktop..."
  for _ in $(seq 1 60); do docker ps >/dev/null 2>&1 && break; sleep 2; done
  docker ps >/dev/null 2>&1 || fail "Docker Desktop is not running. Start it and try again."
fi

if command -v git >/dev/null 2>&1 && [ -d .git ]; then
  echo "Getting the latest version..."
  git pull --ff-only || echo "Could not pull (local changes?). Using the code as it is."
fi

echo "Building and starting HiringPlug (the first time takes a few minutes)..."
docker compose -f deploy/docker-compose.yml up -d --build --remove-orphans || fail "Docker could not start HiringPlug (see above)."

echo "Waiting for the app..."
for _ in $(seq 1 60); do curl -sf http://localhost:3000 >/dev/null && break; sleep 2; done
open http://localhost:3000
echo "HiringPlug is running at http://localhost:3000 (API docs: http://localhost:8080/docs)."
sleep 3
