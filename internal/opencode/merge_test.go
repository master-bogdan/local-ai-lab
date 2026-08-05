package opencode_test

import (
	"encoding/json"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/config"
	"github.com/master-bogdan/local-ai-lab/internal/opencode"
)

func TestMergePreservesUserConfigAndReplacesOnlyOwnedEntries(t *testing.T) {
	existing := []byte(`{
  "theme": "catppuccin",
  "provider": {
    "anthropic": {"options": {"timeout": 30000}},
    "ollama": {"name": "stale"}
  },
  "mcp": {"other": {"type": "local", "command": ["other"]}}
}`)

	merged, err := opencode.Merge(existing, "/repo/.local/bin/local-ai-lab", config.Installation{
		Models: []string{"qwen3.5:9b"}, ModelProfile: "coding", ContextLength: 32768,
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	var document map[string]any
	if err := json.Unmarshal(merged, &document); err != nil {
		t.Fatalf("decode merged config: %v", err)
	}
	if document["theme"] != "catppuccin" {
		t.Fatal("unrelated top-level setting was not preserved")
	}
	if document["share"] != "disabled" {
		t.Fatalf("conversation sharing is not disabled: %#v", document["share"])
	}
	providers := document["provider"].(map[string]any)
	if _, exists := providers["anthropic"]; !exists {
		t.Fatal("unrelated provider was not preserved")
	}
	mcp := document["mcp"].(map[string]any)
	if _, exists := mcp["other"]; !exists {
		t.Fatal("unrelated MCP server was not preserved")
	}
	localAI := mcp["local-ai"].(map[string]any)
	command := localAI["command"].([]any)
	if command[0] != "/repo/.local/bin/local-ai-lab" || command[1] != "mcp" {
		t.Fatalf("unexpected local AI MCP command: %#v", command)
	}
}

func TestMergeAcceptsJSONCWithoutDroppingSettings(t *testing.T) {
	existing := []byte(`{
  // Keep the user's preferred theme.
  "theme": "system",
  "autoupdate": true,
}`)

	merged, err := opencode.Merge(existing, "/repo/.local/bin/local-ai-lab", config.Installation{
		Models: []string{"qwen3.5:9b"}, ModelProfile: "coding", ContextLength: 32768,
	})
	if err != nil {
		t.Fatalf("merge JSONC: %v", err)
	}

	var document map[string]any
	if err := json.Unmarshal(merged, &document); err != nil {
		t.Fatalf("decode merged config: %v", err)
	}
	if document["theme"] != "system" || document["autoupdate"] != true {
		t.Fatalf("JSONC settings were not preserved: %#v", document)
	}
}

func TestMergeUsesWorkloadPrimaryAndConfiguredContext(t *testing.T) {
	merged, err := opencode.Merge(nil, "/repo/.local/bin/local-ai-lab", config.Installation{
		Models: []string{
			"qwen3.5:9b",
			"gpt-oss:20b",
			"gemma3:12b",
			"qwen3-embedding:4b",
		},
		ModelProfile: "complete", ContextLength: 32768,
	})
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any
	if err := json.Unmarshal(merged, &document); err != nil {
		t.Fatal(err)
	}
	if document["model"] != "ollama/gpt-oss:20b" {
		t.Fatalf("primary model = %q", document["model"])
	}
	if document["small_model"] != "ollama/qwen3.5:9b" {
		t.Fatalf("small model = %q", document["small_model"])
	}
	provider := document["provider"].(map[string]any)["ollama"].(map[string]any)
	models := provider["models"].(map[string]any)
	if _, exists := models["qwen3-embedding:4b"]; exists {
		t.Fatal("embedding model exposed as OpenCode chat model")
	}
	for name, value := range models {
		limit := value.(map[string]any)["limit"].(map[string]any)
		if limit["context"] != float64(32768) {
			t.Fatalf("%s context limit = %#v", name, limit["context"])
		}
	}
}
