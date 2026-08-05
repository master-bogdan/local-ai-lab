package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/app"
	"github.com/master-bogdan/local-ai-lab/internal/comfy"
	"github.com/master-bogdan/local-ai-lab/internal/config"
	"github.com/master-bogdan/local-ai-lab/internal/hardware"
	"github.com/master-bogdan/local-ai-lab/internal/opencode"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

func (r Runner) models(ctx context.Context, args []string) error {
	action := ""
	if len(args) > 0 {
		action = args[0]
	} else {
		selected, err := r.terminal.Select("Models", []ui.Option{
			{Label: "List installed models", Description: "Show local Ollama inventory", Value: "list"},
			{Label: "Install model", Description: "Download from Ollama registry", Value: "install"},
			{Label: "Remove model", Description: "Permanently free model storage", Value: "remove"},
			{Label: "Back", Description: "Return to control center", Value: "cancel"},
		}, "list")
		if err != nil {
			return err
		}
		action = selected
	}
	switch action {
	case "list":
		if err := r.startOllama(ctx); err != nil {
			return err
		}
		return r.executeInstalled(ctx, labruntime.ModelListPlan)
	case "install":
		return r.installModel(ctx)
	case "remove":
		return r.removeModel(ctx)
	case "cancel":
		return nil
	default:
		return fmt.Errorf("unknown models action %q", action)
	}
}

func (r Runner) installModel(ctx context.Context) error {
	installation, err := r.store.Load()
	if err != nil {
		return err
	}
	name, err := r.terminal.Input("Ollama model", "qwen3.5:9b")
	if err != nil {
		return err
	}
	name, err = cleanModelName(name)
	if err != nil {
		return err
	}
	confirmed, err := r.terminal.Confirm("Download "+name+"?", false)
	if err != nil || !confirmed {
		return err
	}
	if err := r.startOllama(ctx); err != nil {
		return err
	}
	if err := r.executor.Execute(ctx, labruntime.ModelPullPlan(installation, name)); err != nil {
		return err
	}
	if !contains(installation.Models, name) {
		installation.Models = append(installation.Models, name)
	}
	return r.store.Save(installation)
}

func (r Runner) removeModel(ctx context.Context) error {
	installation, err := r.store.Load()
	if err != nil {
		return err
	}
	defaultName := ""
	if len(installation.Models) > 0 {
		defaultName = installation.Models[len(installation.Models)-1]
	}
	name, err := r.terminal.Input("Model to remove", defaultName)
	if err != nil {
		return err
	}
	name, err = cleanModelName(name)
	if err != nil {
		return err
	}
	confirmed, err := r.terminal.Confirm("Permanently remove "+name+"?", false)
	if err != nil || !confirmed {
		return err
	}
	if err := r.startOllama(ctx); err != nil {
		return err
	}
	if err := r.executor.Execute(ctx, labruntime.ModelRemovePlan(installation, name)); err != nil {
		return err
	}
	installation.Models = without(installation.Models, name)
	return r.store.Save(installation)
}

func (r Runner) startOllama(ctx context.Context) error {
	installation, err := r.store.Load()
	if err != nil {
		return err
	}
	if err := r.executor.Execute(ctx, labruntime.OllamaStartPlan(installation)); err != nil {
		return err
	}
	r.terminal.Warn("Ollama remains running. Return to the main menu to stop services.")
	return nil
}

func (r Runner) setupComfy(ctx context.Context) error {
	installation, err := r.store.Load()
	if err != nil {
		return err
	}
	if installation.Modules.ComfyUI {
		r.terminal.Info("ComfyUI is already configured.")
		return nil
	}
	if installation.Platform == "linux" && installation.Runtime != "cuda" && installation.Runtime != "rocm" {
		return fmt.Errorf("ComfyUI automation does not support experimental %s runtime", installation.Runtime)
	}
	r.terminal.Heading("Image generation setup")
	r.terminal.Info("Runtime is separate from core installation.")
	confirmed, err := r.terminal.Confirm("Install ComfyUI runtime?", false)
	if err != nil || !confirmed {
		return err
	}
	if err := r.executor.Execute(ctx, labruntime.ComfyInstallPlan(installation)); err != nil {
		return err
	}
	if installation.Platform == "darwin" {
		if err := r.initializeComfyDesktop(ctx, installation); err != nil {
			return err
		}
	}
	installation.Modules.ComfyUI = true
	if err := r.store.Save(installation); err != nil {
		return err
	}
	r.terminal.Success("ComfyUI runtime configured.")
	return r.installComfyStarter(ctx, installation)
}

func (r Runner) imageGeneration(ctx context.Context) error {
	for {
		installation, err := r.store.Load()
		if err != nil {
			return err
		}
		if !installation.Modules.ComfyUI {
			confirmed, confirmErr := r.terminal.Confirm("Configure ComfyUI image generation?", false)
			if confirmErr != nil || !confirmed {
				return confirmErr
			}
			return r.setupComfy(ctx)
		}
		choice, selectErr := r.terminal.Select("Image generation", []ui.Option{
			{Label: "Start ComfyUI", Description: "Start only the image runtime", Value: "start"},
			{Label: "Stop ComfyUI", Description: "Stop the image runtime and preserve outputs", Value: "stop"},
			{Label: "Preview latest image", Description: "Render the newest output in this terminal", Value: "preview"},
			{Label: "Install starter models", Description: "Download and verify the recommended image pack", Value: "models"},
			{Label: "Back", Description: "Return to optional setup", Value: "back"},
		}, "start")
		if selectErr != nil {
			return selectErr
		}
		switch choice {
		case "start":
			if err := r.executor.Execute(ctx, labruntime.ComfyStartPlan(installation)); err != nil {
				return err
			}
			r.terminal.Warn("ComfyUI remains running until you explicitly stop services.")
		case "stop":
			if err := r.executor.Execute(ctx, labruntime.ComfyStopPlan(installation)); err != nil {
				return err
			}
			r.terminal.Success("ComfyUI stopped. Generated images are preserved.")
		case "preview":
			path, err := latestGeneratedImage(filepath.Join(installation.DataDir, "services", "comfyui", "output"))
			if err != nil {
				r.terminal.Info("No generated image is available yet.")
				continue
			}
			if err := r.terminal.PreviewImage(path); err != nil {
				return err
			}
		case "models":
			if err := r.installComfyStarter(ctx, installation); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func latestGeneratedImage(root string) (string, error) {
	var latest string
	var latestTime time.Time
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".png", ".jpg", ".jpeg":
		default:
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		if latest == "" || info.ModTime().After(latestTime) {
			latest, latestTime = path, info.ModTime()
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if latest == "" {
		return "", os.ErrNotExist
	}
	return latest, nil
}

func (r Runner) initializeComfyDesktop(ctx context.Context, installation config.Installation) error {
	if err := r.executor.Execute(ctx, labruntime.ComfyStartPlan(installation)); err != nil {
		return err
	}
	r.terminal.Info("Complete Comfy Desktop first-run setup and select MPS acceleration.")
	if err := r.terminal.Pause("Press Enter after Comfy Desktop setup finishes"); err != nil {
		return err
	}
	confirmed, err := r.terminal.Confirm("Quit Comfy Desktop and continue configuration?", false)
	if err != nil {
		return err
	}
	if !confirmed {
		return errors.New("quit Comfy Desktop, then retry image generation from optional setup")
	}
	return r.executor.Execute(ctx, labruntime.ComfyStopPlan(installation))
}

func (r Runner) installComfyStarter(ctx context.Context, installation config.Installation) error {
	report, err := hardware.NewDetector(hardware.HostSystem{}).Detect(ctx, installation.DataDir)
	if err != nil {
		return err
	}
	pack := comfy.StarterPack(report)
	r.terminal.Info("Recommended starter: %s (%s, %.1f GiB)", pack.Name, pack.License, float64(pack.TotalBytes())/float64(hardware.GiB))
	confirmed, err := r.terminal.Confirm("Download and verify this starter pack?", true)
	if err != nil || !confirmed {
		return err
	}
	modelRoot := filepath.Join(installation.DataDir, "models", "comfyui")
	downloader := comfy.NewDownloader(&http.Client{Timeout: 0})
	err = r.terminal.RunTask(ctx, "Download "+pack.Name, "Starter pack verified", func(taskContext context.Context, output io.Writer) error {
		lastPercent := uint64(101)
		progress := func(asset string, downloaded, total uint64) {
			if total == 0 {
				return
			}
			percent := downloaded * 100 / total
			if percent/10 != lastPercent/10 {
				fmt.Fprintf(output, "%s  %d%%\n", asset, percent)
				lastPercent = percent
			}
		}
		return downloader.Install(taskContext, pack, modelRoot, progress)
	})
	if err != nil {
		return err
	}
	if installation.Platform == "darwin" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		backup, err := comfy.ConfigureDesktop(homeDir, modelRoot, time.Now())
		if err != nil {
			return err
		}
		if backup != "" {
			r.terminal.Info("ComfyUI config backup: %s", backup)
		}
	}
	if !contains(installation.Modules.ComfyModels, pack.Name) {
		installation.Modules.ComfyModels = append(installation.Modules.ComfyModels, pack.Name)
	}
	if err := r.store.Save(installation); err != nil {
		return err
	}
	r.terminal.Success("ComfyUI starter pack installed and verified.")
	return nil
}

func (r Runner) setupMonitoring(ctx context.Context) error {
	installation, err := r.store.Load()
	if err != nil {
		return err
	}
	if installation.Services.Monitoring {
		r.terminal.Info("Monitoring is already configured. Return to the main menu and choose Start.")
		return nil
	}
	r.terminal.Heading("Monitoring setup")
	r.terminal.Info("Includes Prometheus, cAdvisor, Grafana, recording rules, alerts, and a default dashboard.")
	confirmed, err := r.terminal.Confirm("Download and enable the monitoring stack?", false)
	if err != nil || !confirmed {
		return err
	}
	if err := r.executor.Execute(ctx, labruntime.MonitoringInstallPlan(installation)); err != nil {
		return err
	}
	installation.Services.Monitoring = true
	if err := r.store.Save(installation); err != nil {
		return err
	}
	r.terminal.Success("Monitoring configured. Services remain stopped; return to the main menu and choose Start.")
	return nil
}

func (r Runner) optionalModules(ctx context.Context) error {
	for {
		choice, err := r.terminal.Select("Optional modules", []ui.Option{
			{Label: "Image generation", Description: "Configure ComfyUI and starter models", Value: "comfy"},
			{Label: "Monitoring dashboard", Description: "Prometheus metrics and Grafana dashboards", Value: "monitoring"},
			{Label: "OpenCode", Description: "Connect terminal coding agent to local models", Value: "opencode"},
			{Label: "Back", Description: "Return to control center", Value: "exit"},
		}, "exit")
		if err != nil {
			return err
		}
		switch choice {
		case "comfy":
			if err := r.imageGeneration(ctx); err != nil {
				return err
			}
		case "opencode":
			if err := r.configureOpenCode(); err != nil {
				return err
			}
		case "monitoring":
			if err := r.setupMonitoring(ctx); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func (r Runner) configureOpenCode() error {
	installation, err := r.store.Load()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("opencode"); err != nil {
		r.terminal.Warn("OpenCode is not installed. Install it manually, then retry OpenCode from Optional setup.")
		return nil
	}
	path, err := opencodePath()
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	merged, err := opencode.Merge(existing, r.integrationBinaryPath(), installation)
	if err != nil {
		return err
	}
	preview := fmt.Sprintf("PATH\n%s\n\nCONFIG\n%s", path, string(merged))
	confirmed, err := r.terminal.Review(
		"OpenCode config preview", "Existing config is backed up before writing",
		preview, "Write config", false,
	)
	if err != nil || !confirmed {
		return err
	}
	backup, err := opencode.WriteWithBackup(path, merged, time.Now())
	if err != nil {
		return err
	}
	installation.Modules.OpenCode = true
	if err := r.store.Save(installation); err != nil {
		return err
	}
	if backup != "" {
		r.terminal.Info("Backup: %s", backup)
	}
	r.terminal.Success("OpenCode configured for local models.")
	return nil
}

func (r Runner) delete(ctx context.Context) error {
	installation, err := r.store.Load()
	if err != nil {
		return err
	}
	choice, err := r.terminal.Select("Deletion mode", []ui.Option{
		{Label: "Full deletion", Description: "Remove installation, models, cache, indexes, and service data", Value: "full"},
		{Label: "Partial deletion", Description: "Choose individual data categories", Value: "partial"},
		{Label: "Cancel", Description: "Return without deleting anything", Value: "cancel"},
	}, "cancel")
	if err != nil || choice == "cancel" {
		return err
	}
	plan, err := r.deletionPlan(choice, installation)
	if err != nil {
		return err
	}
	if err := r.previewDeletion(plan); err != nil {
		return err
	}
	confirmation, err := r.terminal.Input("Type "+plan.Confirmation, "")
	if err != nil {
		return err
	}
	if err := r.executor.Execute(ctx, labruntime.StopPlan(installation)); err != nil {
		return fmt.Errorf("stop services before deletion: %w", err)
	}
	title := "Delete selected data"
	if choice == "full" {
		title = "Delete Local AI Lab data"
	}
	if err := r.terminal.RunTask(ctx, title, "Deletion complete", func(_ context.Context, output io.Writer) error {
		for _, path := range plan.Paths {
			fmt.Fprintf(output, "delete  %s\n", path)
		}
		return plan.Execute(confirmation)
	}); err != nil {
		return err
	}
	if choice == "partial" && plan.Includes(app.DeleteModels) {
		if err := app.PrepareDataLayout(installation.DataDir); err != nil {
			return err
		}
		installation.Models = nil
		installation.Modules.ComfyModels = nil
		if err := r.store.Save(installation); err != nil {
			return fmt.Errorf("update model inventory after deletion: %w", err)
		}
	}
	return nil
}

func (r Runner) deletionPlan(mode string, installation config.Installation) (app.DeletionPlan, error) {
	if mode == "full" {
		return app.FullDeletionPlan(r.store.PointerPath(), installation), nil
	}
	selected, err := r.terminal.MultiSelect("Select data to delete", []ui.Option{
		{Label: "Models", Description: "Ollama and ComfyUI model files", Value: string(app.DeleteModels)},
		{Label: "Search cache", Description: "Cached public search results", Value: string(app.DeleteCache)},
		{Label: "Knowledge indexes", Description: "Indexed workspace vectors", Value: string(app.DeleteIndexes)},
		{Label: "Service data", Description: "Container state and application databases", Value: string(app.DeleteServiceData)},
	})
	if err != nil {
		return app.DeletionPlan{}, err
	}
	if len(selected) == 0 {
		return app.DeletionPlan{}, errors.New("no deletion categories selected")
	}
	categories := make([]app.DeletionCategory, 0, len(selected))
	for _, category := range selected {
		categories = append(categories, app.DeletionCategory(category))
	}
	return app.PartialDeletionPlan(installation, categories), nil
}

func (r Runner) previewDeletion(plan app.DeletionPlan) error {
	targets, err := plan.Preview()
	if err != nil {
		return err
	}
	var body strings.Builder
	for _, target := range targets {
		fmt.Fprintf(&body, "%s\n  %s\n\n", target.Path, formatDataBytes(target.SizeBytes))
	}
	body.WriteString("OpenCode config and backups are never deleted.")
	return r.terminal.Show("Deletion preview", "Nothing is deleted until exact text confirmation", strings.TrimSpace(body.String()))
}

func formatDataBytes(size int64) string {
	const gib = 1024 * 1024 * 1024
	if size >= gib {
		return fmt.Sprintf("%.1f GiB", float64(size)/gib)
	}
	const mib = 1024 * 1024
	if size >= mib {
		return fmt.Sprintf("%.1f MiB", float64(size)/mib)
	}
	return fmt.Sprintf("%d bytes", size)
}

func without(values []string, unwanted string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value != unwanted {
			filtered = append(filtered, value)
		}
	}
	return filtered
}
