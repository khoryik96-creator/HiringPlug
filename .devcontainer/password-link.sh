#!/usr/bin/env bash
# Prints a fresh link for the codespace's first owner to choose a password
# (valid 7 days), for a forgotten password or an expired first link.
set -euo pipefail
cd "$(dirname "$0")/.."
set -a
# shellcheck disable=SC1091
. /workspaces/.hiringplug-codespace.env
set +a
if [ -n "${CODESPACE_NAME:-}" ]; then
  export HP_PUBLIC_URL="https://${CODESPACE_NAME}-3000.${GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN:-app.github.dev}"
fi
compose=(docker compose -f deploy/docker-compose.yml)
owner=$("${compose[@]}" exec -T postgres psql -U postgres -d hiringplug -Atc \
  "SELECT id FROM app_user WHERE role = 'owner' ORDER BY created_at LIMIT 1")
"${compose[@]}" run --rm -T bootstrap /app/hp password-link -user "$owner" -valid 168h
