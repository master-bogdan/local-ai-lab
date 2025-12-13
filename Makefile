.DEFAULT_GOAL := help

.PHONY: help install start clear models-install models-remove

help:
	@echo ""
	@echo "local-ai-lab"
	@echo ""
	@echo "Targets:"
	@echo "  make install         - Combine envs + dirs + monitoring + pull models"
	@echo "  make start           - Install if needed, then docker compose up (foreground)"
	@echo "  make clear           - Remove everything installed (down -v + generated files)"
	@echo "  make models-install  - Pull models only (optional args via MODELS=...)"
	@echo "  make models-remove   - Remove models only (required args via MODELS=...)"
	@echo ""
	@echo "Examples:"
	@echo "  make start"
	@echo "  make models-install MODELS='deepseek-r1:14b qwen2.5-coder:14b'"
	@echo "  make models-remove  MODELS='deepseek-r1:8b'"
	@echo ""

install:
	./scripts/install.sh

start:
	./scripts/start.sh

clear:
	./scripts/clear.sh

# Optional helpers (only if you keep models scripts):
models-install:
	./scripts/models-install.sh $(MODELS)

models-remove:
	@if [ -z "$(MODELS)" ]; then \
		echo "ERROR: MODELS is required. Example:"; \
		echo "  make models-remove MODELS='deepseek-r1:8b'"; \
		exit 1; \
	fi
	./scripts/models-remove.sh $(MODELS)
