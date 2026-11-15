#!/bin/sh
# On the server: get the latest version and restart HiringPlug with it.
#   sh deploy/update-server.sh
# Your data is kept (the database, files and backups live in Docker volumes and
# the backups folder). Settings come from deploy/.env (see
# docs/playbooks/deploy-to-a-server.md).
set -eu
cd "$(dirname "$0")/.."

if [ ! -f deploy/.env ]; then
  echo "deploy/.env is missing: create it first (docs/playbooks/deploy-to-a-server.md)." >&2
  exit 1
fi

echo "Getting the latest version..."
git pull --ff-only

echo "Building and restarting (a few minutes)..."
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml up -d --build --remove-orphans

echo "Waiting for the app..."
domain=$(sed -n 's/^HP_DOMAIN=//p' deploy/.env | tr -d '"' | tail -n 1)
i=0
while [ "$i" -lt 60 ]; do
  if curl -sf -o /dev/null "https://$domain/health/ready"; then
    echo "HiringPlug is running at https://$domain"
    exit 0
  fi
  i=$((i + 1))
  sleep 5
done
echo "The app did not answer at https://$domain/health/ready: see 'docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml logs api'." >&2
exit 1
