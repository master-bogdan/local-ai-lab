#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

SENTINEL=".installed"

if [[ ! -f "$SENTINEL" ]]; then
  echo "[start] Not installed. Running install..."
  ./scripts/install.sh
fi

# If .env is missing for any reason, re-run install (it regenerates .env first)
if [[ ! -f .env ]]; then
  echo "[start] Missing .env. Re-running install..."
  ./scripts/install.sh
fi

echo "[start] docker compose up (foreground, no detach)"
docker compose up
