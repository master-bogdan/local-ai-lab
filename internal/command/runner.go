package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/master-bogdan/local-ai-lab/internal/app"
	"github.com/master-bogdan/local-ai-lab/internal/config"
	"github.com/master-bogdan/local-ai-lab/internal/hardware"
	"github.com/master-bogdan/local-ai-lab/internal/opencode"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

const windowsMessage = `WINDOWS DETECTED

You took a wrong turn.

This is a local AI lab, not a Windows troubleshooting department.
No Windows. No WSL. No compatibility hacks. Windows users are unwelcome.

Install Linux, use a Mac, or close this terminal with dignity.`

type Runner struct {
	repoDir  string
	store    config.Store
	terminal *ui.Terminal
	executor planExecutor
}

type windowsUnsupportedError struct{}

func (windowsUnsupportedError) Error() string { return windowsMessage }

func NewRunner(repoDir string, terminal *ui.Terminal) Runner {
	return Runner{
		repoDir:  repoDir,
		store:    config.NewStore(repoDir),
		terminal: terminal,
		executor: interactivePlanExecutor{repoDir: repoDir, terminal: terminal},
	}
}

func (r Runner) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return r.start(ctx)
	}
	switch args[0] {
	case "start":
		return r.start(ctx)
	case "mcp":
		return r.runMCP(ctx)
	case "help", "--help", "-h":
		r.help()
		return nil
	default:
		return fmt.Errorf("direct command %q is unavailable; run make start and use the control center", args[0])
	}
}

func (r Runner) installCore(ctx context.Context, experimental bool) error {
	detector := hardware.NewDetector(hardware.HostSystem{})
	preparedExecutor := dependencyExecutor{terminal: r.terminal, next: r.executor}
	installer := app.NewInstaller(r.store, detector, r.terminal, preparedExecutor)
	err := installer.Run(ctx, app.InstallOptions{AllowExperimental: experimental})
	if errors.Is(err, app.ErrUnsupportedHardware) && (hardware.HostSystem{}).OS() == "windows" {
		return windowsUnsupportedError{}
	}
	if err != nil {
		return err
	}
	r.terminal.Success("Local AI Lab installed. All services are stopped.")
	r.terminal.Info("Returning to main menu. Choose Start when ready.")
	return nil
}

func (r Runner) start(ctx context.Context) error {
	return r.home(ctx)
}

func (r Runner) startWorkload(ctx context.Context) error {
	if err := r.chooseAndStartWorkload(ctx); err != nil {
		return err
	}
	return r.dashboard(ctx)
}

func (r Runner) chooseAndStartWorkload(ctx context.Context) error {
	installation, err := r.store.Load()
	if err != nil {
		return err
	}
	workload, err := r.terminal.ChooseWorkload(ctx, defaultWorkload(installation))
	if err != nil {
		return err
	}
	if workloadUsesImages(workload) && !installation.Modules.ComfyUI {
		if err := r.setupComfy(ctx); err != nil {
			return err
		}
	}
	if err := r.confirmNativeSwitch(installation, workload); err != nil {
		return err
	}
	controller := app.NewController(r.store, r.terminal, r.executor)
	if err := controller.StartWorkload(ctx, workload); err != nil {
		return err
	}
	r.terminal.Success("%s workload started", workload)
	return nil
}

func (r Runner) stop(ctx context.Context) error {
	installation, err := r.store.Load()
	if err != nil {
		return err
	}
	if installation.Platform == "darwin" {
		confirmed, err := r.terminal.Confirm("Quit Ollama and ComfyUI native apps?", false)
		if err != nil || !confirmed {
			return err
		}
	}
	if err := r.executor.Execute(ctx, labruntime.StopPlan(installation)); err != nil {
		return err
	}
	r.terminal.Success("Services stopped. Data and models preserved.")
	return nil
}

func (r Runner) dashboard(ctx context.Context) error {
	for {
		installation, err := r.store.Load()
		if err != nil {
			return err
		}
		summary, err := r.installationSummary(ctx, installation)
		if err != nil {
			return err
		}
		summary.Status = serviceStatus(ctx, installation)
		choice, err := r.terminal.InstallationMenu(summary, dashboardOptions(), "exit")
		if errors.Is(err, ui.ErrInterrupted) {
			done, exitErr := r.exitDashboard(ctx)
			if done || exitErr != nil {
				return exitErr
			}
			continue
		}
		if err != nil {
			return err
		}
		if done, err := r.dashboardAction(ctx, choice); done || err != nil {
			return err
		}
	}
}

func (r Runner) dashboardAction(ctx context.Context, choice string) (bool, error) {
	switch choice {
	case "status":
		if err := r.executeInstalled(ctx, labruntime.StatusPlan); err != nil {
			return false, err
		}
		installation, err := r.store.Load()
		if err != nil {
			return false, err
		}
		return false, r.showURLs(installation, true)
	case "switch":
		return false, r.chooseAndStartWorkload(ctx)
	case "models":
		return false, r.models(ctx, []string{"list"})
	case "optional":
		return false, r.optionalModules(ctx)
	case "delete":
		return false, r.delete(ctx)
	case "exit":
		return r.exitDashboard(ctx)
	default:
		return false, nil
	}
}

func (r Runner) exitDashboard(ctx context.Context) (bool, error) {
	var choice string
	for {
		selected, err := r.terminal.Select("Services are still running", []ui.Option{
			{Label: "Leave running and exit", Description: "Services continue in the background", Value: "leave"},
			{Label: "Stop services and exit", Description: "Cleanly stop runtimes; preserve data", Value: "stop"},
			{Label: "Cancel", Description: "Return to control center", Value: "cancel"},
		}, "stop")
		if errors.Is(err, ui.ErrInterrupted) {
			continue
		}
		if err != nil {
			return false, err
		}
		choice = selected
		break
	}
	switch choice {
	case "leave":
		r.terminal.Warn("Services remain active. Run make start later and choose Stop services.")
		return true, nil
	case "stop":
		return true, r.stop(ctx)
	default:
		return false, nil
	}
}

func dashboardOptions() []ui.Option {
	return []ui.Option{
		{Label: "Service status and URLs", Description: "Health and localhost endpoints", Value: "status"},
		{Label: "Switch workload", Description: "Change active model or image runtime", Value: "switch"},
		{Label: "Installed models", Description: "Show Ollama model inventory", Value: "models"},
		{Label: "Optional modules", Description: "ComfyUI, monitoring, and OpenCode", Value: "optional"},
		{Label: "Delete data", Description: "Review partial or full deletion", Value: "delete"},
		{Label: "Exit dashboard", Description: "Choose whether services keep running", Value: "exit"},
	}
}

func (r Runner) showURLs(installation config.Installation, warnRunning bool) error {
	var body strings.Builder
	if warnRunning {
		body.WriteString("Services run in background. Return through make start to stop them later.\n\n")
	}
	body.WriteString("Ollama      http://127.0.0.1:11434\n")
	if installation.Services.WebUI {
		body.WriteString("Open WebUI  http://127.0.0.1:3000\n")
	}
	if installation.Services.Search {
		body.WriteString("SearXNG     http://127.0.0.1:8088\n")
	}
	if installation.Services.Knowledge {
		body.WriteString("Qdrant      http://127.0.0.1:6333/dashboard\n")
	}
	if installation.Modules.ComfyUI {
		body.WriteString("ComfyUI     http://127.0.0.1:8188\n")
	}
	if installation.Services.Monitoring {
		fmt.Fprintf(&body, "Grafana     http://127.0.0.1:3002/d/local-ai-lab/local-ai-lab-overview\n            admin / %s\n", installation.Secrets.Grafana)
		body.WriteString("Prometheus  http://127.0.0.1:9090\n")
		body.WriteString("cAdvisor    http://127.0.0.1:8080\n")
	}
	return r.terminal.Show("Local service URLs", "Localhost access only", strings.TrimSpace(body.String()))
}

func (r Runner) executeInstalled(ctx context.Context, plan func(config.Installation) labruntime.Plan) error {
	installation, err := r.store.Load()
	if err != nil {
		return err
	}
	return r.executor.Execute(ctx, plan(installation))
}

func defaultWorkload(installation config.Installation) labruntime.Workload {
	if installation.Workload == "" {
		return labruntime.Coding
	}
	return labruntime.Workload(installation.Workload)
}

func workloadUsesImages(workload labruntime.Workload) bool {
	return workload == labruntime.Images || workload == labruntime.Both
}

func (r Runner) confirmNativeSwitch(installation config.Installation, workload labruntime.Workload) error {
	if installation.Platform != "darwin" || installation.Workload == "" {
		return nil
	}
	previous := labruntime.Workload(installation.Workload)
	willQuitApp := previous != workload && workload != labruntime.Both
	if !willQuitApp {
		return nil
	}
	confirmed, err := r.terminal.Confirm("Switching workloads will quit the other native AI app. Continue?", true)
	if err != nil {
		return err
	}
	if !confirmed {
		return errors.New("workload switch cancelled")
	}
	return nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (r Runner) help() {
	r.terminal.Welcome()
	r.terminal.Info("Run make start from the repository to open the interactive control center.")
}

func (r Runner) doctor(ctx context.Context) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dataDir := config.DefaultDataDir(homeDir)
	if installation, loadErr := r.store.Load(); loadErr == nil {
		dataDir = installation.DataDir
	}
	report, err := hardware.NewDetector(hardware.HostSystem{}).Detect(ctx, dataDir)
	if err != nil {
		return err
	}
	if report.OS == hardware.Windows {
		return windowsUnsupportedError{}
	}
	assessment := hardware.Assess(report, true)
	if !assessment.Supported {
		return fmt.Errorf("%w: %s", app.ErrUnsupportedHardware, assessment.Reason)
	}
	if err := r.checkDependencies(ctx, report); err != nil {
		return err
	}
	body := fmt.Sprintf(
		"OS        %s %s %s\nGPU       %s\nRuntime   %s\nVRAM      %d MiB\nRAM       %d GiB\nFree disk %d GiB\nTier      %s\nImages    %s",
		report.OS, report.Distro, report.DistroVersion, report.GPU.Name, report.GPU.Runtime,
		report.GPU.VRAMBytes/(1024*1024), report.MemoryBytes/hardware.GiB,
		report.DiskBytes/hardware.GiB, assessment.Tier, r.terminal.ImageProtocol(),
	)
	return r.terminal.Show("Hardware compatibility", "Supported local AI host", body)
}

func (r Runner) checkDependencies(ctx context.Context, report hardware.Report) error {
	for _, command := range []string{"docker", "go", "make"} {
		if _, err := exec.LookPath(command); err != nil {
			return fmt.Errorf("missing required command %s", command)
		}
	}
	if err := exec.CommandContext(ctx, "docker", "compose", "version").Run(); err != nil {
		return errors.New("missing Docker Compose plugin")
	}
	if report.OS == hardware.MacOS {
		if _, err := exec.LookPath("ollama"); err != nil {
			return errors.New("native Ollama is missing; run brew install --cask ollama")
		}
	}
	return nil
}

func cleanModelName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " \t\n") {
		return "", errors.New("invalid model name")
	}
	return name, nil
}

func opencodePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return opencode.ConfigPath(homeDir), nil
}

func repoBinaryPath(repoDir string) string {
	return filepath.Join(repoDir, ".local", "bin", "localai")
}
