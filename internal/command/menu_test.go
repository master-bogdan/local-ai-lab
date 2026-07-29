package command

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/config"
	"github.com/master-bogdan/local-ai-lab/internal/hardware"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

func TestSummaryFromInstallationIncludesConfiguredCapabilities(t *testing.T) {
	installation := config.Installation{
		DataDir: "/data/local-ai-lab", Runtime: "cuda",
		Models:       []string{"qwen3.5:9b", "qwen3-embedding:0.6b"},
		ModelProfile: "coding", ContextLength: 32768,
		Services: config.Services{Search: true, Knowledge: true, WebUI: true, Monitoring: true},
		Modules:  config.Modules{ComfyUI: true, OpenCode: true}, Workload: "coding",
	}
	report := hardware.Report{
		OS: "linux", Distro: "fedora", DistroVersion: "42",
		MemoryBytes: 64 * hardware.GiB, DiskBytes: 312 * hardware.GiB,
		GPU: hardware.GPU{Name: "RTX 4090 Laptop GPU", VRAMBytes: 16 * hardware.GiB},
	}

	summary := summaryFromInstallation(installation, report, 84*hardware.GiB)

	if summary.Platform != "fedora 42" || summary.Models != 2 {
		t.Fatalf("unexpected summary identity: %#v", summary)
	}
	if summary.Services != "Search, Knowledge, Web UI, Monitoring" {
		t.Fatalf("unexpected services: %q", summary.Services)
	}
	if summary.Modules != "ComfyUI, OpenCode" || summary.Workload != "coding" {
		t.Fatalf("unexpected modules/workload: %#v", summary)
	}
	if summary.ModelProfile != "coding" || summary.ContextLength != 32768 {
		t.Fatalf("recommendation metadata missing: %#v", summary)
	}
}

func TestEmbeddingModelUsesSavedChoiceAndSafeFallback(t *testing.T) {
	if got := embeddingModel(config.Installation{EmbeddingModel: "qwen3-embedding:4b"}); got != "qwen3-embedding:4b" {
		t.Fatalf("saved embedding model ignored: %q", got)
	}
	if got := embeddingModel(config.Installation{}); got != "qwen3-embedding:0.6b" {
		t.Fatalf("fallback embedding model = %q", got)
	}
}

func TestInstalledMenuOwnsAllUserActions(t *testing.T) {
	wanted := []string{"start", "status", "logs", "models", "optional", "index", "stop", "delete", "doctor", "exit"}
	options := installedMenuOptions()
	if len(options) != len(wanted) {
		t.Fatalf("installed menu has %d options, want %d: %#v", len(options), len(wanted), options)
	}
	for index, value := range wanted {
		if options[index].Value != value {
			t.Fatalf("option %d = %q, want %q", index, options[index].Value, value)
		}
	}
}

func TestOnboardingMenuOffersSafePreInstallActions(t *testing.T) {
	wanted := []string{"install", "experimental", "doctor", "requirements", "exit"}
	options := onboardingMenuOptions()
	if len(options) != len(wanted) {
		t.Fatalf("onboarding menu has %d options, want %d: %#v", len(options), len(wanted), options)
	}
	for index, value := range wanted {
		if options[index].Value != value {
			t.Fatalf("option %d = %q, want %q", index, options[index].Value, value)
		}
	}
}

func TestStatusEndpointsFollowSelectedWorkload(t *testing.T) {
	installation := config.Installation{
		Workload: "coding",
		Services: config.Services{Search: true, Knowledge: true, WebUI: true, Monitoring: true},
		Modules:  config.Modules{ComfyUI: true},
	}
	endpoints := statusEndpoints(installation)
	for _, name := range []string{"Ollama", "SearXNG", "Qdrant", "Open WebUI", "Grafana", "Prometheus", "cAdvisor"} {
		if _, ok := endpoints[name]; !ok {
			t.Fatalf("coding status does not probe %s: %#v", name, endpoints)
		}
	}
	if _, ok := endpoints["ComfyUI"]; ok {
		t.Fatalf("coding status unexpectedly probes ComfyUI: %#v", endpoints)
	}
}

func TestRunningOrPartialInstallationConfirmsExit(t *testing.T) {
	for _, status := range []string{"Running", "Partial (2/4 services responding)"} {
		if !shouldConfirmExit(status) {
			t.Fatalf("status %q does not confirm exit", status)
		}
	}
	if shouldConfirmExit("Ready") {
		t.Fatal("stopped installation should exit without service confirmation")
	}
}

func TestStatusProbesConfiguredEnginesOutsideCurrentWorkload(t *testing.T) {
	installation := config.Installation{Workload: "infrastructure", Modules: config.Modules{ComfyUI: true}}
	endpoints := statusProbeEndpoints(installation)
	for _, name := range []string{"Ollama", "ComfyUI"} {
		if _, ok := endpoints[name]; !ok {
			t.Fatalf("status does not detect independently started %s: %#v", name, endpoints)
		}
	}
}

func TestDirectLifecycleCommandsAreRejected(t *testing.T) {
	terminal := ui.NewTerminal(bytes.NewBuffer(nil), &bytes.Buffer{})
	runner := NewRunner(t.TempDir(), terminal)

	err := runner.Run(context.Background(), []string{"models", "list"})
	if err == nil || !strings.Contains(err.Error(), "make start") {
		t.Fatalf("direct command error = %v, want make start guidance", err)
	}
}
