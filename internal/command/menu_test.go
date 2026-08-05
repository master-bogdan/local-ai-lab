package command

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/config"
	"github.com/master-bogdan/local-ai-lab/internal/distribution"
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
	wanted := []string{
		"start", "status", "logs", "models", "optional", "index",
		"stop", "application", "delete", "doctor", "exit",
	}
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
	wanted := []string{"install", "experimental", "doctor", "requirements", "application", "exit"}
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

func TestApplicationMenuOwnsReleaseAndUninstallLifecycle(t *testing.T) {
	wanted := []string{"update", "rollback", "uninstall", "back"}
	options := applicationMenuOptions()
	if len(options) != len(wanted) {
		t.Fatalf("application menu has %d options, want %d: %#v", len(options), len(wanted), options)
	}
	for index, value := range wanted {
		if options[index].Value != value {
			t.Fatalf("option %d = %q, want %q", index, options[index].Value, value)
		}
	}
}

func TestUninstallMenuOffersPreservingAndAbsoluteRemoval(t *testing.T) {
	wanted := []string{"application", "full", "absolute", "cancel"}
	options := uninstallMenuOptions()
	if len(options) != len(wanted) {
		t.Fatalf("uninstall menu has %d options, want %d: %#v", len(options), len(wanted), options)
	}
	for index, value := range wanted {
		if options[index].Value != value {
			t.Fatalf("option %d = %q, want %q", index, options[index].Value, value)
		}
	}
}

func TestInterruptedInstallMenuOffersRecoveryWithoutSilentDeletion(t *testing.T) {
	wanted := []string{"resume", "remove", "exit"}
	options := interruptedInstallOptions()
	if len(options) != len(wanted) {
		t.Fatalf("interrupted install menu has %d options, want %d", len(options), len(wanted))
	}
	for index, value := range wanted {
		if options[index].Value != value {
			t.Fatalf("option %d = %q, want %q", index, options[index].Value, value)
		}
	}
}

func TestReinstallMenuOffersReuseOrFreshSetup(t *testing.T) {
	wanted := []string{"reuse", "custom"}
	options := reinstallOptions(distribution.Receipt{})
	if len(options) != len(wanted) {
		t.Fatalf("reinstall menu has %d options, want %d", len(options), len(wanted))
	}
	for index, value := range wanted {
		if options[index].Value != value {
			t.Fatalf("option %d = %q, want %q", index, options[index].Value, value)
		}
	}
}

func TestReceiptPreservesPreviousChoicesWithoutCurrentLab(t *testing.T) {
	installedAt := time.Date(2026, time.July, 1, 12, 0, 0, 0, time.UTC)
	existing := distribution.Receipt{
		Schema: 1, LastVersion: "v0.1.0", InstalledAt: installedAt,
		DataDir: "/data/local-ai-lab", Workload: "coding",
		Models: []string{"qwen3.5:9b"}, Services: []string{"search"},
	}
	now := installedAt.Add(24 * time.Hour)

	got := buildReceipt(existing, nil, "v0.2.0", true, now)

	if got.LastVersion != "v0.2.0" || got.DataDir != existing.DataDir ||
		len(got.Models) != 1 || got.Models[0] != "qwen3.5:9b" {
		t.Fatalf("preserved receipt = %#v", got)
	}
	if got.UninstalledAt == nil || !got.UninstalledAt.Equal(now) {
		t.Fatalf("uninstall timestamp = %#v, want %s", got.UninstalledAt, now)
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
	appRoot := t.TempDir()
	layout := distribution.UserLayout(t.TempDir(), "linux", nil)
	runner := NewRunner(appRoot, "/tmp/local-ai-lab", layout, "dev", "test", terminal)

	err := runner.Run(context.Background(), []string{"models", "list"})
	if err == nil || !strings.Contains(err.Error(), "local-ai-lab") {
		t.Fatalf("direct command error = %v, want local-ai-lab guidance", err)
	}
}
