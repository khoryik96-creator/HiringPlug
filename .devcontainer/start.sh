#!/usr/bin/env bash
# Starts HiringPlug in a codespace: its own random secrets (made once), the
# Docker stack, and on the first start an agency whose owner chooses a
# password from a link. Runs at every start of the codespace; safe to rerun.
set -euo pipefail
cd "$(dirname "$0")/.."

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
fi

for _ in $(seq 60); do docker info >/dev/null 2>&1 && break; sleep 2; done
compose=(docker compose -f deploy/docker-compose.yml)
echo "Starting HiringPlug (the first start builds it: about 10 minutes)..."
"${compose[@]}" up -d --build --wait postgres mailpit api worker ai-worker web

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
