#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

usage() {
  echo "Usage:"
  echo "  ./scripts/models-remove.sh <model1> <model2> ..."
  echo "  ./scripts/models-remove.sh --all"
}

if [[ ${#@} -eq 0 ]]; then
  usage
  exit 1
fi

if [[ "${1:-}" == "--all" ]]; then
  echo "[models-remove] Listing models..."
  docker compose run --rm ollama list || true

  echo ""
  read -r -p "Remove ALL models from Ollama? Type 'YES' to continue: " confirm
  if [[ "$confirm" != "YES" ]]; then
    echo "[models-remove] Aborted."
    exit 0
  fi

  # Remove everything listed (skip header)
  docker compose run --rm --entrypoint sh ollama -lc \
    "ollama list | awk 'NR>1 {print \$1}' | while read -r m; do \
       [ -z \"\$m\" ] && continue; \
       echo \"  -> removing \$m\"; \
       ollama rm \"\$m\"; \
     done"
  echo "[models-remove] Done."
  exit 0
fi

echo "[models-remove] Removing specified models..."
for model in "$@"; do
  echo "  -> $model"
  docker compose run --rm ollama rm "$model"
done

echo "[models-remove] Done."
