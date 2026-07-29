package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/master-bogdan/local-ai-lab/internal/app"
	"github.com/master-bogdan/local-ai-lab/internal/config"
	"github.com/master-bogdan/local-ai-lab/internal/hardware"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

func (r Runner) home(ctx context.Context) error {
	for {
		installation, err := r.store.Load()
		if errors.Is(err, config.ErrNotInstalled) {
			exit, menuErr := r.onboardingMenu(ctx)
			if exit {
				return nil
			}
			if menuErr != nil {
				if err := r.showActionError(menuErr); err != nil {
					return err
				}
			}
			continue
		}
		if err != nil {
			return err
		}
		exit, menuErr := r.installedMenu(ctx, installation)
		if exit {
			return nil
		}
		if menuErr != nil {
			if err := r.showActionError(menuErr); err != nil {
				return err
			}
		}
	}
}

func (r Runner) showActionError(actionErr error) error {
	if errors.Is(actionErr, ui.ErrInterrupted) {
		return nil
	}
	err := r.terminal.Show("Action failed", "No further changes were made", actionErr.Error())
	if errors.Is(err, ui.ErrInterrupted) {
		return nil
	}
	return err
}

func (r Runner) onboardingMenu(ctx context.Context) (bool, error) {
	choice, err := r.terminal.OnboardingMenu(onboardingMenuOptions(), "install")
	if errors.Is(err, ui.ErrInterrupted) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	switch choice {
	case "install", "experimental":
		if err := r.installIntroduction(); err != nil {
			return false, err
		}
		allowExperimental := choice == "experimental"
		if allowExperimental {
			r.terminal.Warn("Experimental Vulkan/XPU support may be slower or incompatible with some GPUs.")
		}
		if err := r.installCore(ctx, allowExperimental); err != nil {
			if errors.Is(err, app.ErrInstallCancelled) {
				r.terminal.Info("Installation cancelled. Nothing was started.")
				return false, nil
			}
			return false, err
		}
		return false, nil
	case "doctor":
		return false, r.doctor(ctx)
	case "requirements":
		return false, r.showRequirements()
	default:
		return true, nil
	}
}

func (r Runner) installedMenu(ctx context.Context, installation config.Installation) (bool, error) {
	summary, err := r.installationSummary(ctx, installation)
	if err != nil {
		return false, err
	}
	summary.Status = serviceStatus(ctx, installation)
	choice, err := r.terminal.InstallationMenu(summary, installedMenuOptions(), "start")
	if errors.Is(err, ui.ErrInterrupted) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	switch choice {
	case "start":
		return true, r.startWorkload(ctx)
	case "status":
		if err := r.executeInstalled(ctx, labruntime.StatusPlan); err != nil {
			return false, err
		}
		return false, r.showURLs(installation, false)
	case "logs":
		return false, r.executeInstalled(ctx, labruntime.LogsPlan)
	case "models":
		return false, r.models(ctx, nil)
	case "optional":
		return false, r.optionalModules(ctx)
	case "index":
		workspace, err := os.Getwd()
		if err != nil {
			return false, err
		}
		workspace, err = r.terminal.Input("Workspace path", workspace)
		if err != nil {
			return false, err
		}
		return false, r.indexWorkspace(ctx, workspace)
	case "stop":
		return false, r.stop(ctx)
	case "delete":
		return false, r.delete(ctx)
	case "doctor":
		return false, r.doctor(ctx)
	default:
		if shouldConfirmExit(summary.Status) {
			return r.exitDashboard(ctx)
		}
		return true, nil
	}
}

func onboardingMenuOptions() []ui.Option {
	return []ui.Option{
		{Label: "Install Local AI Lab", Description: "Recommended setup for detected hardware", Value: "install"},
		{Label: "Experimental GPU setup", Description: "Vulkan or Intel XPU; compatibility varies", Value: "experimental"},
		{Label: "Check hardware", Description: "Inspect GPU, memory, disk, and required tools", Value: "doctor"},
		{Label: "View requirements", Description: "Supported platforms and minimum hardware", Value: "requirements"},
		{Label: "Exit", Description: "Leave without making changes", Value: "exit"},
	}
}

func installedMenuOptions() []ui.Option {
	return []ui.Option{
		{Label: "Start or switch workload", Description: "Coding, images, infrastructure, or both", Value: "start"},
		{Label: "Service status and URLs", Description: "Health and localhost endpoints", Value: "status"},
		{Label: "Follow service logs", Description: "Stream container output until Ctrl+C", Value: "logs"},
		{Label: "Manage models", Description: "List, download, or remove Ollama models", Value: "models"},
		{Label: "Optional setup", Description: "ComfyUI, monitoring, and OpenCode", Value: "optional"},
		{Label: "Index a workspace", Description: "Add repository knowledge for local agents", Value: "index"},
		{Label: "Stop services", Description: "Stop runtimes and preserve all data", Value: "stop"},
		{Label: "Delete data", Description: "Review partial or full deletion", Value: "delete"},
		{Label: "Check hardware", Description: "Re-run compatibility diagnostics", Value: "doctor"},
		{Label: "Exit", Description: "Leave control center", Value: "exit"},
	}
}

func (r Runner) installIntroduction() error {
	body := strings.Join([]string{
		"Local models, coding agents, web search, workspace knowledge, images, and monitoring.",
		"",
		"Services bind to localhost. No paid API is required.",
		"Nothing runs without confirmation.",
		"Installation finishes with all services stopped.",
		"",
		"Setup checks hardware and disk, collects choices, previews changes, then installs.",
	}, "\n")
	return r.terminal.Show("Private AI workstation setup", "Review before continuing", body)
}

func (r Runner) showRequirements() error {
	body := strings.Join([]string{
		"LINUX",
		"Fedora, Ubuntu, or Arch Linux",
		"Discrete GPU with 6+ GiB VRAM",
		"16+ GiB system RAM",
		"",
		"MACOS",
		"Apple Silicon, 24+ GiB unified RAM",
		"Homebrew",
		"",
		"UNSUPPORTED",
		"CPU-only systems and Intel Macs",
		"Windows and WSL",
		"",
		"Internet is required for packages, containers, models, and public web search.",
	}, "\n")
	return r.terminal.Show("System requirements", "Supported local AI hosts", body)
}

func (r Runner) installationSummary(ctx context.Context, installation config.Installation) (ui.InstallationSummary, error) {
	report, err := hardware.NewDetector(hardware.HostSystem{}).Detect(ctx, installation.DataDir)
	if err != nil {
		return ui.InstallationSummary{}, err
	}
	dataBytes, err := app.DirectorySize(installation.DataDir)
	if err != nil {
		return ui.InstallationSummary{}, fmt.Errorf("measure installation data: %w", err)
	}
	return summaryFromInstallation(installation, report, uint64(dataBytes)), nil
}

func summaryFromInstallation(installation config.Installation, report hardware.Report, dataBytes uint64) ui.InstallationSummary {
	platform := strings.TrimSpace(strings.Join([]string{report.Distro, report.DistroVersion}, " "))
	if platform == "" {
		platform = string(report.OS)
	}
	workload := installation.Workload
	if workload == "" {
		workload = "Not started"
	}
	return ui.InstallationSummary{
		Status: "Ready", DataDir: installation.DataDir,
		DataBytes: dataBytes, FreeDiskBytes: report.DiskBytes,
		Platform: platform, GPU: report.GPU.Name, Runtime: installation.Runtime,
		VRAMBytes: report.GPU.VRAMBytes, RAMBytes: report.MemoryBytes,
		Models: len(installation.Models), Services: serviceNames(installation.Services),
		Modules: moduleNames(installation.Modules), Workload: workload,
		ModelProfile: installation.ModelProfile, ContextLength: installation.ContextLength,
	}
}

func serviceNames(services config.Services) string {
	names := make([]string, 0, 4)
	if services.Search {
		names = append(names, "Search")
	}
	if services.Knowledge {
		names = append(names, "Knowledge")
	}
	if services.WebUI {
		names = append(names, "Web UI")
	}
	if services.Monitoring {
		names = append(names, "Monitoring")
	}
	if len(names) == 0 {
		return "Ollama only"
	}
	return strings.Join(names, ", ")
}

func moduleNames(modules config.Modules) string {
	names := make([]string, 0, 2)
	if modules.ComfyUI {
		names = append(names, "ComfyUI")
	}
	if modules.OpenCode {
		names = append(names, "OpenCode")
	}
	if len(names) == 0 {
		return "None"
	}
	return strings.Join(names, ", ")
}
