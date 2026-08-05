.DEFAULT_GOAL := start

CLI := .local/bin/local-ai-lab
GO_FILES := $(shell find cmd internal -name '*.go' -type f)
RETIRED_TARGETS := install stop status logs doctor models comfy monitoring opencode index delete build help

.PHONY: start test check $(RETIRED_TARGETS)

$(CLI): $(GO_FILES) go.mod go.sum
	@mkdir -p .local/bin
	@go build -trimpath \
		-ldflags "-X github.com/master-bogdan/local-ai-lab/internal/buildinfo.Commit=$$(git rev-parse --short=12 HEAD)" \
		-o $(CLI) ./cmd/local-ai-lab

start: $(CLI)
	@LOCAL_AI_LAB_APP_ROOT="$(CURDIR)" $(CLI) start

$(RETIRED_TARGETS):
	@printf '%s\n' 'Direct commands were removed. Run make start and use the interactive menu.'
	@exit 2

test:
	@go test ./...

check:
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal; exit 1)
	@go vet ./...
	@go tool staticcheck ./...
	@go test -race ./...
	@go tool govulncheck ./...
