package opencode

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tailscale/hujson"

	"github.com/master-bogdan/local-ai-lab/internal/config"
)

func Merge(existing []byte, binaryPath string, installation config.Installation) ([]byte, error) {
	document := make(map[string]any)
	if len(existing) > 0 {
		standardized, err := hujson.Standardize(existing)
		if err != nil {
			return nil, fmt.Errorf("parse existing OpenCode config: %w", err)
		}
		if err := json.Unmarshal(standardized, &document); err != nil {
			return nil, fmt.Errorf("parse existing OpenCode config: %w", err)
		}
	}
	document["$schema"] = "https://opencode.ai/config.json"
	document["share"] = "disabled"
	providers := object(document, "provider")
	providers["ollama"] = ollamaProvider(installation.Models, installation.ContextLength)
	mcp := object(document, "mcp")
	mcp["local-ai"] = map[string]any{
		"type":    "local",
		"command": []string{binaryPath, "mcp"},
		"enabled": true,
		"timeout": 10000,
	}
	permissions := object(document, "permission")
	permissions["edit"] = "ask"
	permissions["bash"] = "ask"
	permissions["external_directory"] = "ask"
	permissions["local-ai_*"] = "allow"
	setDefaultModels(document, installation.Models, installation.ModelProfile)
	merged, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode OpenCode config: %w", err)
	}
	return append(merged, '\n'), nil
}

func object(document map[string]any, key string) map[string]any {
	if existing, ok := document[key].(map[string]any); ok {
		return existing
	}
	created := make(map[string]any)
	document[key] = created
	return created
}

func ollamaProvider(modelNames []string, contextLength int) map[string]any {
	if contextLength == 0 {
		contextLength = 32768
	}
	models := make(map[string]any)
	for _, name := range modelNames {
		if strings.Contains(name, "embedding") {
			continue
		}
		models[name] = map[string]any{
			"name": displayName(name),
			"limit": map[string]any{
				"context": contextLength,
				"output":  min(8192, contextLength/2),
			},
		}
	}
	return map[string]any{
		"npm":  "@ai-sdk/openai-compatible",
		"name": "Ollama (Local AI Lab)",
		"options": map[string]any{
			"baseURL": "http://127.0.0.1:11434/v1",
		},
		"models": models,
	}
}

func setDefaultModels(document map[string]any, modelNames []string, profile string) {
	chatModels := make([]string, 0, len(modelNames))
	for _, name := range modelNames {
		if !strings.Contains(name, "embedding") {
			chatModels = append(chatModels, name)
		}
	}
	if len(chatModels) == 0 {
		return
	}
	small := preferred(chatModels, []string{"qwen3.5:4b", "qwen3.5:9b"})
	primaryOrder := []string{"gpt-oss:120b", "gpt-oss:20b", "qwen3.6:35b", "qwen3.6:27b", "qwen3.5:9b"}
	switch profile {
	case "coding":
		primaryOrder = []string{"qwen3-coder-next", "devstral-small-2:24b", "qwen3.6:35b", "qwen3.6:27b", "qwen3.5:9b"}
	case "vision":
		primaryOrder = []string{"gemma3:12b", "qwen3.5:9b"}
	}
	document["small_model"] = "ollama/" + small
	document["model"] = "ollama/" + preferred(chatModels, primaryOrder)
}

func preferred(available, preference []string) string {
	for _, wanted := range preference {
		for _, candidate := range available {
			if candidate == wanted {
				return candidate
			}
		}
	}
	return available[0]
}

func displayName(name string) string {
	return strings.NewReplacer("qwen", "Qwen ", ":", " ", "gpt-oss", "GPT-OSS").Replace(name)
}
