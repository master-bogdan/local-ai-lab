#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

DEFAULT_MODELS=(
  "deepseek-r1:14b"
  "qwen2.5:14b"
  "qwen2.5-coder:14b"
  "llama3.1:8b"
  "qwen3-vl:8b"
  "phi4-reasoning:latest"
  "qwen3:14b"
)

MODELS=("$@")
if [[ ${#MODELS[@]} -eq 0 ]]; then
  MODELS=("${DEFAULT_MODELS[@]}")
fi

echo "[models-install] Pulling models into the Ollama volume..."
for model in "${MODELS[@]}"; do
  echo "  -> $model"
  docker compose run --rm --entrypoint sh ollama -lc \
    "ollama serve >/tmp/ollama-serve.log 2>&1 & pid=\$!; \
     sleep 2; \
     ollama pull '$model'; \
     kill \$pid"
done

echo "[models-install] Done."
