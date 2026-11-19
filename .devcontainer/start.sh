#!/usr/bin/env bash
# Starts HiringPlug in a codespace: its own random secrets (made once), the
# Docker stack, and on the first start an agency whose owner chooses a
# password from a link. Runs at every start of the codespace; safe to rerun.
set -euo pipefail
cd "$(dirname "$0")/.."

# Updates pass an inherited lock so the checkout, Compose and initial agency
# setup are serialized with the Codespace's postStartCommand.
if [[ -z "${HIRINGPLUG_CODESPACE_START_FD:-}" ]]; then
  exec {HIRINGPLUG_CODESPACE_START_FD}>../.hiringplug-codespace.lock
fi
flock "$HIRINGPLUG_CODESPACE_START_FD"

secrets=/workspaces/.hiringplug-codespace.env # outside the repository: never committed
if [ ! -f "$secrets" ]; then
  rnd() { head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 40; }
  umask 077
  {
    for v in HP_AUTH_JWT_SECRET HP_FILES_URL_SECRET HP_MAIL_KEYS_SECRET HP_AI_KEYS_SECRET HP_WEBHOOK_KEYS_SECRET \
      HP_DB_ADMIN_PASSWORD HP_DB_API_PASSWORD HP_DB_AI_PASSWORD; do
      echo "$v=$(rnd)"
    done
  } > "$secrets"
fi
set -a
# shellcheck disable=SC1090
. "$secrets"
set +a
if [ -n "${CODESPACE_NAME:-}" ]; then
  export HP_PUBLIC_URL="https://${CODESPACE_NAME}-3000.${GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN:-app.github.dev}"
  grep -q '^HP_PUBLIC_URL=' "$secrets" || echo "HP_PUBLIC_URL=$HP_PUBLIC_URL" >> "$secrets"
fi
# Docker Compose reads deploy/.env (ignored by git) on its own, so a plain
# "docker compose -f deploy/docker-compose.yml ..." uses the same passwords
# the database was created with (without them the set-up step is refused).
[ -e deploy/.env ] || ln -s "$secrets" deploy/.env

for _ in $(seq 60); do docker info >/dev/null 2>&1 && break; sleep 2; done
compose=(docker compose -f deploy/docker-compose.yml)
echo "Starting HiringPlug (the first start builds it: about 10 minutes)..."
if ! "${compose[@]}" up -d --build --wait postgres mailpit api worker ai-worker web; then
  echo
  echo "HiringPlug did not start. What the database set-up (bootstrap) said:"
  "${compose[@]}" logs --no-color --tail 30 bootstrap || true
  echo "Run this script again to retry: bash .devcontainer/start.sh"
  exit 1
fi

link_file=CODESPACE-SIGN-IN.txt
agencies=$("${compose[@]}" exec -T postgres psql -U postgres -d hiringplug -Atc "SELECT count(*) FROM tenant")
if [ "$agencies" = "0" ]; then
  email=$(git config user.email || true)
  name=$(git config user.name || true)
  "${compose[@]}" run --rm -T bootstrap /app/hp tenant -name "HiringPlug trial" \
    -owner-email "${email:-owner@example.com}" -owner-name "${name:-Owner}" > "$link_file"
fi
echo
echo "HiringPlug is running: ${HP_PUBLIC_URL:-http://localhost:3000}"
[ -f "$link_file" ] && cat "$link_file"
echo "Sign-in details are kept in $link_file. A new password link: bash .devcontainer/password-link.sh"
