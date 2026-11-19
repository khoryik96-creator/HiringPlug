#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ -z "${CODESPACE_NAME:-}" ]]; then
  echo "Run this updater in the HiringPlug GitHub Codespace." >&2
  exit 1
fi

if [[ "$(git branch --show-current)" != main ]]; then
  echo "This Codespace is on a different branch. Switch to main before running the updater." >&2
  exit 1
fi

git pull --ff-only origin main
exec bash .devcontainer/start.sh
