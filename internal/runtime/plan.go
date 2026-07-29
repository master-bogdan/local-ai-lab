package runtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/config"
)

type StepKind string

const (
	PullImages  StepKind = "pull-images"
	StartOllama StepKind = "start-ollama"
	PullModel   StepKind = "pull-model"
	StopAll     StepKind = "stop-all"
	WaitHTTP    StepKind = "wait-http"
)

type Step struct {
	Kind    StepKind
	Command string
	Args    []string
	Model   string
	URL     string
}

type Plan struct {
	Steps        []Step
	LoopbackOnly bool
	Workload     Workload
	Environment  []string
	finalRunning bool
}

type Workload string

const comfyDesktopApp = "Comfy Desktop"

const (
	Coding         Workload = "coding"
	Images         Workload = "images"
	Infrastructure Workload = "infrastructure"
	Both           Workload = "both"
)

func InstallPlan(installation config.Installation) Plan {
	services := selectedServices(installation, Coding)
	steps := make([]Step, 0, len(installation.Models)+4)
	if len(services) > 0 {
		steps = append(steps, Step{Kind: PullImages, Command: "docker", Args: append([]string{"compose", "pull"}, services...)})
	}
	if installation.Platform == "darwin" {
		steps = append(steps,
			Step{Command: "open", Args: []string{"-a", "Ollama"}},
			Step{Kind: WaitHTTP, URL: "http://127.0.0.1:11434/api/version"},
		)
	} else {
		steps = append(steps, Step{Kind: StartOllama, Command: "docker", Args: []string{"compose", "up", "-d", "--wait", "ollama"}})
	}
	for _, model := range installation.Models {
		steps = append(steps, modelPullStep(installation.Platform, model))
	}
	if installation.Platform == "darwin" {
		steps = append(steps, Step{Kind: StopAll, Command: "osascript", Args: []string{"-e", `quit app "Ollama"`}})
	} else {
		steps = append(steps, Step{Kind: StopAll, Command: "docker", Args: []string{"compose", "down", "--remove-orphans"}})
	}
	return Plan{Steps: steps, LoopbackOnly: true, Environment: composeEnvironment(installation)}
}

func (p Plan) LeavesServicesRunning() bool {
	return p.finalRunning
}

func (p Plan) PullsModel(name string) bool {
	for _, step := range p.Steps {
		if step.Kind == PullModel && step.Model == name {
			return true
		}
	}
	return false
}

func StartPlan(installation config.Installation, workload Workload) Plan {
	services := selectedServices(installation, workload)
	steps := make([]Step, 0, 3)
	if len(services) > 0 {
		steps = append(steps, Step{Command: "docker", Args: append([]string{"compose", "up", "-d", "--wait"}, services...)})
	}
	if installation.Platform == "darwin" && (workload == Coding || workload == Both) {
		steps = append([]Step{{Command: "open", Args: []string{"-a", "Ollama"}}}, steps...)
	}
	if installation.Platform == "darwin" && (workload == Images || workload == Both) {
		steps = append(steps, Step{Command: "open", Args: []string{"-a", comfyDesktopApp}})
	}
	return Plan{
		Steps: steps, LoopbackOnly: true, Workload: workload,
		Environment: composeEnvironment(installation), finalRunning: true,
	}
}

func selectedServices(installation config.Installation, workload Workload) []string {
	services := make([]string, 0, 4)
	if workload == Coding || workload == Both {
		if installation.Platform != "darwin" {
			services = append(services, "ollama")
		}
	}
	if installation.Services.Search {
		services = append(services, "searxng")
	}
	if installation.Services.Knowledge {
		services = append(services, "qdrant")
	}
	if installation.Services.WebUI && (workload == Coding || workload == Both) {
		services = append(services, "open-webui")
	}
	if installation.Services.Monitoring {
		services = append(services, "cadvisor", "prometheus", "grafana")
	}
	if installation.Modules.ComfyUI && installation.Platform != "darwin" && (workload == Images || workload == Both) {
		services = append(services, "comfyui")
	}
	return services
}

func StopPlan(installation config.Installation) Plan {
	steps := []Step{{Command: "docker", Args: []string{"compose", "down", "--remove-orphans"}}}
	if installation.Platform == "darwin" {
		steps = append(steps, Step{Command: "osascript", Args: []string{"-e", `quit app "Ollama"`}})
		if installation.Modules.ComfyUI {
			steps = append(steps, Step{Command: "osascript", Args: []string{"-e", `quit app "Comfy Desktop"`}})
		}
	}
	return Plan{Steps: steps, LoopbackOnly: true, Environment: composeEnvironment(installation)}
}

func SwitchPlan(installation config.Installation, previous, next Workload) Plan {
	plan := StartPlan(installation, next)
	if previous == next || previous == "" {
		return plan
	}
	stops := make([]Step, 0, 2)
	if usesCoding(previous) && !usesCoding(next) {
		stops = append(stops, stopEngine(installation.Platform, "Ollama", "ollama"))
	}
	if installation.Modules.ComfyUI && usesImages(previous) && !usesImages(next) {
		stops = append(stops, stopEngine(installation.Platform, comfyDesktopApp, "comfyui"))
	}
	plan.Steps = append(stops, plan.Steps...)
	return plan
}

func usesCoding(workload Workload) bool {
	return workload == Coding || workload == Both
}

func usesImages(workload Workload) bool {
	return workload == Images || workload == Both
}

func stopEngine(platform, appName, service string) Step {
	if platform == "darwin" {
		return Step{Command: "osascript", Args: []string{"-e", fmt.Sprintf(`quit app %q`, appName)}}
	}
	return Step{Command: "docker", Args: []string{"compose", "stop", service}}
}

func StatusPlan(installation config.Installation) Plan {
	return Plan{
		Steps:        []Step{{Command: "docker", Args: []string{"compose", "ps"}}},
		LoopbackOnly: true, Environment: composeEnvironment(installation),
	}
}

func LogsPlan(installation config.Installation) Plan {
	return Plan{
		Steps:        []Step{{Command: "docker", Args: []string{"compose", "logs", "-f", "--tail", "100"}}},
		LoopbackOnly: true, Environment: composeEnvironment(installation),
	}
}

func OllamaStartPlan(installation config.Installation) Plan {
	steps := []Step{{Command: "docker", Args: []string{"compose", "up", "-d", "--wait", "ollama"}}}
	if installation.Platform == "darwin" {
		steps = []Step{
			{Command: "open", Args: []string{"-a", "Ollama"}},
			{Kind: WaitHTTP, URL: "http://127.0.0.1:11434/api/version"},
		}
	}
	return Plan{
		Steps: steps, LoopbackOnly: true, Environment: composeEnvironment(installation), finalRunning: true,
	}
}

func KnowledgeStartPlan(installation config.Installation) Plan {
	services := []string{"ollama", "qdrant"}
	steps := []Step{{Command: "docker", Args: append([]string{"compose", "up", "-d", "--wait"}, services...)}}
	if installation.Platform == "darwin" {
		steps = []Step{
			{Command: "open", Args: []string{"-a", "Ollama"}},
			{Kind: WaitHTTP, URL: "http://127.0.0.1:11434/api/version"},
			{Command: "docker", Args: []string{"compose", "up", "-d", "--wait", "qdrant"}},
		}
	}
	return Plan{
		Steps: steps, LoopbackOnly: true, Environment: composeEnvironment(installation), finalRunning: true,
	}
}

func ModelListPlan(installation config.Installation) Plan {
	return Plan{
		Steps:        []Step{ollamaStep(installation.Platform, "list")},
		LoopbackOnly: true, Environment: composeEnvironment(installation),
	}
}

func ModelPullPlan(installation config.Installation, model string) Plan {
	return Plan{
		Steps:        []Step{modelPullStep(installation.Platform, model)},
		LoopbackOnly: true, Environment: composeEnvironment(installation),
	}
}

func ModelRemovePlan(installation config.Installation, model string) Plan {
	return Plan{
		Steps:        []Step{ollamaStep(installation.Platform, "rm", model)},
		LoopbackOnly: true, Environment: composeEnvironment(installation),
	}
}

func ComfyInstallPlan(installation config.Installation) Plan {
	if installation.Platform == "darwin" {
		return Plan{Steps: []Step{{Command: "brew", Args: []string{"install", "--cask", "comfy"}}}, LoopbackOnly: true}
	}
	return Plan{
		Steps:        []Step{{Command: "docker", Args: []string{"compose", "build", "comfyui"}}},
		LoopbackOnly: true, Environment: composeEnvironment(installation),
	}
}

func MonitoringInstallPlan(installation config.Installation) Plan {
	return Plan{
		Steps: []Step{{
			Kind: PullImages, Command: "docker",
			Args: []string{"compose", "pull", "cadvisor", "prometheus", "grafana"},
		}},
		LoopbackOnly: true,
		Environment:  composeEnvironment(installation),
	}
}

func ComfyStartPlan(installation config.Installation) Plan {
	if installation.Platform == "darwin" {
		return Plan{Steps: []Step{{Command: "open", Args: []string{"-a", comfyDesktopApp}}}, LoopbackOnly: true}
	}
	return Plan{
		Steps:        []Step{{Command: "docker", Args: []string{"compose", "up", "-d", "--wait", "comfyui"}}},
		LoopbackOnly: true, Environment: composeEnvironment(installation), finalRunning: true,
	}
}

func ComfyStopPlan(installation config.Installation) Plan {
	var step Step
	if installation.Platform == "darwin" {
		step = Step{Command: "osascript", Args: []string{"-e", `quit app "Comfy Desktop"`}}
	} else {
		step = Step{Command: "docker", Args: []string{"compose", "stop", "comfyui"}}
	}
	return Plan{Steps: []Step{step}, LoopbackOnly: true, Environment: composeEnvironment(installation)}
}

func ollamaStep(platform string, args ...string) Step {
	if platform == "darwin" {
		return Step{Command: "ollama", Args: args}
	}
	composeArgs := []string{"compose", "exec", "-T", "ollama", "ollama"}
	return Step{Command: "docker", Args: append(composeArgs, args...)}
}

func composeEnvironment(installation config.Installation) []string {
	files := []string{"deploy/compose.yaml"}
	switch {
	case installation.Platform == "darwin":
		files = append(files, "deploy/compose.macos.yaml")
	case installation.Runtime == "rocm":
		files = append(files, "deploy/compose.rocm.yaml")
	case installation.Runtime == "vulkan" || installation.Runtime == "xpu":
		files = append(files, "deploy/compose.vulkan.yaml")
	default:
		files = append(files, "deploy/compose.nvidia.yaml")
	}
	contextLength := installation.ContextLength
	if contextLength == 0 {
		contextLength = 32768
		if installation.Runtime == "vulkan" || installation.Runtime == "xpu" {
			contextLength = 16384
		}
	}
	embeddingModel := installation.EmbeddingModel
	if embeddingModel == "" {
		embeddingModel = "qwen3-embedding:0.6b"
	}
	return []string{
		"LOCAL_AI_DATA_DIR=" + installation.DataDir,
		"COMPOSE_FILE=" + strings.Join(files, string(os.PathListSeparator)),
		"SEARXNG_SECRET=" + installation.Secrets.SearXNG,
		"GRAFANA_PASSWORD=" + installation.Secrets.Grafana,
		fmt.Sprintf("OLLAMA_CONTEXT_LENGTH=%d", contextLength),
		"EMBEDDING_MODEL=" + embeddingModel,
	}
}

type CommandExecutor struct {
	Dir    string
	Stdout io.Writer
	Stderr io.Writer
}

func (e CommandExecutor) Execute(ctx context.Context, plan Plan) error {
	for _, step := range plan.Steps {
		if step.Kind == WaitHTTP {
			if err := waitHTTP(ctx, step.URL, 90*time.Second); err != nil {
				return err
			}
			continue
		}
		command := exec.CommandContext(ctx, step.Command, step.Args...)
		command.Dir = e.Dir
		command.Env = append(os.Environ(), plan.Environment...)
		command.Stdout = e.Stdout
		command.Stderr = e.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("run %s %s: %w", step.Command, strings.Join(step.Args, " "), err)
		}
	}
	return nil
}

func waitHTTP(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode < http.StatusInternalServerError {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("%s did not become ready within %s", url, timeout)
}

func modelPullStep(platform, model string) Step {
	if platform == "darwin" {
		return Step{Kind: PullModel, Command: "ollama", Args: []string{"pull", model}, Model: model}
	}
	return Step{
		Kind:    PullModel,
		Command: "docker",
		Args:    []string{"compose", "exec", "-T", "ollama", "ollama", "pull", model},
		Model:   model,
	}
}
