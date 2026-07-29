package runtime_test

import (
	"strings"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/config"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
)

func TestInstallPlanDownloadsModelsAndLeavesServicesStopped(t *testing.T) {
	installation := config.Installation{
		DataDir:  "/tmp/local-ai-lab",
		Models:   []string{"qwen3.5:9b", "qwen3-embedding:0.6b"},
		Services: config.Services{Search: true, Knowledge: true, WebUI: true},
	}

	plan := labruntime.InstallPlan(installation)

	if plan.LeavesServicesRunning() {
		t.Fatal("install plan must leave all services stopped")
	}
	if !plan.PullsModel("qwen3.5:9b") || !plan.PullsModel("qwen3-embedding:0.6b") {
		t.Fatalf("install plan does not pull selected models: %#v", plan.Steps)
	}
	if !plan.LoopbackOnly {
		t.Fatal("install plan must bind published services to loopback")
	}
}

func TestPlanExportsConfiguredContextAndEmbeddingModel(t *testing.T) {
	installation := config.Installation{
		DataDir:        "/tmp/local-ai-lab",
		Platform:       "linux",
		Runtime:        "cuda",
		ContextLength:  16384,
		EmbeddingModel: "qwen3-embedding:4b",
	}

	plan := labruntime.StartPlan(installation, labruntime.Coding)
	environment := strings.Join(plan.Environment, "\n")

	for _, wanted := range []string{
		"OLLAMA_CONTEXT_LENGTH=16384",
		"EMBEDDING_MODEL=qwen3-embedding:4b",
	} {
		if !strings.Contains(environment, wanted) {
			t.Fatalf("runtime environment is missing %q:\n%s", wanted, environment)
		}
	}
}

func TestMacInstallStartsAndStopsOllamaAroundModelPulls(t *testing.T) {
	installation := config.Installation{
		Platform: "darwin",
		Models:   []string{"qwen3.5:9b"},
	}

	plan := labruntime.InstallPlan(installation)

	if len(plan.Steps) < 4 {
		t.Fatalf("mac install plan is incomplete: %#v", plan.Steps)
	}
	if plan.Steps[0].Command != "open" || plan.Steps[0].Args[1] != "Ollama" {
		t.Fatalf("Ollama is not started before model pulls: %#v", plan.Steps)
	}
	if plan.Steps[1].Kind != labruntime.WaitHTTP || plan.Steps[2].Kind != labruntime.PullModel {
		t.Fatalf("model is not pulled after Ollama starts: %#v", plan.Steps)
	}
	last := plan.Steps[len(plan.Steps)-1]
	if last.Command != "osascript" {
		t.Fatalf("mac install leaves Ollama running: %#v", plan.Steps)
	}
}

func TestSwitchFromBothToInfrastructureStopsBothEngines(t *testing.T) {
	installation := config.Installation{
		Platform: "linux",
		Modules:  config.Modules{ComfyUI: true},
	}

	plan := labruntime.SwitchPlan(installation, labruntime.Both, labruntime.Infrastructure)

	stopped := map[string]bool{}
	for _, step := range plan.Steps {
		if len(step.Args) == 3 && step.Args[0] == "compose" && step.Args[1] == "stop" {
			stopped[step.Args[2]] = true
		}
	}
	if !stopped["ollama"] || !stopped["comfyui"] {
		t.Fatalf("infrastructure switch did not stop both engines: %#v", plan.Steps)
	}
}

func TestMacImageWorkloadStartsComfyDesktop(t *testing.T) {
	installation := config.Installation{
		Platform: "darwin",
		Modules:  config.Modules{ComfyUI: true},
	}

	plan := labruntime.StartPlan(installation, labruntime.Images)

	for _, step := range plan.Steps {
		if step.Command == "open" && len(step.Args) == 2 && step.Args[1] == "Comfy Desktop" {
			return
		}
	}
	t.Fatalf("mac image workload does not launch Comfy Desktop: %#v", plan.Steps)
}

func TestMacCodingWithNoCoreServicesDoesNotStartAllContainers(t *testing.T) {
	installation := config.Installation{Platform: "darwin", Models: []string{"qwen3.5:9b"}}

	for _, plan := range []labruntime.Plan{
		labruntime.InstallPlan(installation),
		labruntime.StartPlan(installation, labruntime.Coding),
	} {
		for _, step := range plan.Steps {
			if step.Command == "docker" {
				t.Fatalf("empty service selection invoked Docker without a service: %#v", plan.Steps)
			}
		}
	}
}

func TestOllamaStartPlanStartsOnlyOllama(t *testing.T) {
	plan := labruntime.OllamaStartPlan(config.Installation{Platform: "linux"})
	if len(plan.Steps) != 1 {
		t.Fatalf("unexpected Ollama start plan: %#v", plan.Steps)
	}
	step := plan.Steps[0]
	if step.Command != "docker" || len(step.Args) != 5 || step.Args[4] != "ollama" {
		t.Fatalf("Ollama start plan targets wrong services: %#v", step)
	}
}

func TestCUDAComfyInstallBuildsLocalImage(t *testing.T) {
	plan := labruntime.ComfyInstallPlan(config.Installation{Platform: "linux", Runtime: "cuda"})
	if len(plan.Steps) != 1 {
		t.Fatalf("unexpected Comfy install plan: %#v", plan.Steps)
	}
	step := plan.Steps[0]
	if step.Command != "docker" || len(step.Args) != 3 || step.Args[1] != "build" || step.Args[2] != "comfyui" {
		t.Fatalf("CUDA ComfyUI does not build the pinned local image: %#v", step)
	}
}

func TestKnowledgeStartPlanStartsOnlyOllamaAndQdrant(t *testing.T) {
	plan := labruntime.KnowledgeStartPlan(config.Installation{Platform: "linux", Services: config.Services{Knowledge: true}})
	if len(plan.Steps) != 1 {
		t.Fatalf("unexpected knowledge start plan: %#v", plan.Steps)
	}
	step := plan.Steps[0]
	want := []string{"compose", "up", "-d", "--wait", "ollama", "qdrant"}
	if step.Command != "docker" || len(step.Args) != len(want) {
		t.Fatalf("unexpected knowledge start step: %#v", step)
	}
	for index := range want {
		if step.Args[index] != want[index] {
			t.Fatalf("knowledge start step = %#v, want %#v", step.Args, want)
		}
	}
}

func TestMacComfyStartPlanOnlyOpensDesktop(t *testing.T) {
	plan := labruntime.ComfyStartPlan(config.Installation{Platform: "darwin"})
	if len(plan.Steps) != 1 {
		t.Fatalf("unexpected Comfy Desktop start plan: %#v", plan.Steps)
	}
	step := plan.Steps[0]
	if step.Command != "open" || len(step.Args) != 2 || step.Args[1] != "Comfy Desktop" {
		t.Fatalf("Comfy Desktop start plan invokes unexpected process: %#v", step)
	}
}

func TestMonitoringInstallPlanPullsOnlyMonitoringServices(t *testing.T) {
	plan := labruntime.MonitoringInstallPlan(config.Installation{Platform: "linux", Runtime: "cuda"})
	if len(plan.Steps) != 1 {
		t.Fatalf("unexpected monitoring install plan: %#v", plan.Steps)
	}
	want := []string{"compose", "pull", "cadvisor", "prometheus", "grafana"}
	step := plan.Steps[0]
	if step.Command != "docker" || len(step.Args) != len(want) {
		t.Fatalf("unexpected monitoring install step: %#v", step)
	}
	for index := range want {
		if step.Args[index] != want[index] {
			t.Fatalf("monitoring install step = %#v, want %#v", step.Args, want)
		}
	}
	if plan.LeavesServicesRunning() {
		t.Fatal("monitoring installation must leave services stopped")
	}
}
