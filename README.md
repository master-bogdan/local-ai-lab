# local-ai-lab

Local AI stack for development and prototyping: Ollama + Open WebUI, ComfyUI, SearXNG (web search for RAG), Bolt.diy, plus monitoring via Prometheus/Grafana/cAdvisor.

## Services

- `ollama` — LLM runtime / model host
- `openwebui-core` — chat UI + RAG
- `searxng` — private meta-search (used by Open WebUI web search)
- `bolt-diy` — Bolt.diy app (optional UI)
- `comfyui` — image generation UI
- `prometheus`, `grafana`, `cadvisor` — monitoring/metrics

## Prerequisites

- Docker + Compose plugin (`docker compose`)
- Optional GPU:
  - Linux (NVIDIA): install `nvidia-container-toolkit`
  - CPU-only: remove `gpus: all` from `docker-compose.yaml` for `ollama`/`comfyui`

## Quick start

Recommended (Makefile):

```bash
make install
make start
```

Scripts:

```bash
./scripts/install.sh
./scripts/start.sh
```

Stop and clean up (preserves Ollama models by default):

```bash
make clear
```

## Default URLs

Defaults come from `config/*.env.example` and are merged into `.env` by `./scripts/install.sh`.

- Open WebUI: `http://localhost:3000`
- Ollama API: `http://localhost:11434`
- ComfyUI: `http://localhost:8188`
- SearXNG: `http://localhost:8088`
- Bolt.diy: `http://localhost:5173`
- Grafana: `http://localhost:3002` (default `admin` / `admin`)
- Prometheus: `http://localhost:9090`
- cAdvisor: `http://localhost:8080`

## Configuration

- Source of truth: `config/*.env` (copy from `config/*.env.example` on first install).
- Install step merges `config/*.env` into root `.env` in a deterministic order (alphabetical by filename, later files override earlier ones).
- Changing `config/*.env` requires re-running `./scripts/install.sh` to regenerate `.env`.

Common settings:

- Ports: `OLLAMA_PORT`, `OPENWEBUI_CORE_PORT`, `COMFYUI_PORT`, `SEARXNG_PORT`, `BOLTDIY_PORT`, `GRAFANA_PORT`, `PROMETHEUS_PORT`, `CADVISOR_PORT`
- Open WebUI ↔ Ollama: `OLLAMA_BASE_URL=http://ollama:11434`
- Open WebUI web search via SearXNG: `ENABLE_RAG_WEB_SEARCH=true` and `SEARXNG_QUERY_URL=http://searxng:8080/search?q=<query>`

Generated runtime artifacts:

- `.env` — generated from `config/*.env`
- `.installed` — sentinel used by `./scripts/start.sh`
- `monitoring/prometheus/prometheus.yml` and `monitoring/grafana/provisioning/datasources/datasource.yml` — created if missing (never overwritten)
- `data/searxng/settings.yml` — generated on install (includes JSON search when `SEARXNG_ENABLE_JSON=true`)

## Models (Ollama)

- Pull default set: `make models-install` (or edit `DEFAULT_MODELS` in `./scripts/models-install.sh`)
- Pull specific models: `make models-install MODELS='qwen2.5-coder:14b llama3.1:8b'`
- Remove models: `make models-remove MODELS='qwen2.5-coder:14b'`
- Wipe everything including models: `make clear-with-models` (or `./scripts/clear.sh --wipe-models`)

## Ops

- Logs: `docker compose logs -f`
- Restart a service: `docker compose restart openwebui-core`

## Notes

- `cadvisor` runs `privileged: true` to read host/container metrics; don’t expose it publicly.
- If you want a stable SearXNG secret across re-installs, set `SEARXNG_SECRET_KEY` in `config/searxng.env` (otherwise install will generate one into `.env`).

## License

MIT — see `LICENSE`.
