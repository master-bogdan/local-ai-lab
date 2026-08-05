package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/app"
	"github.com/master-bogdan/local-ai-lab/internal/config"
	"github.com/master-bogdan/local-ai-lab/internal/distribution"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

var errApplicationExit = errors.New("application lifecycle completed; exit required")

func applicationMenuOptions() []ui.Option {
	return []ui.Option{
		{Label: "Check for updates", Description: "Download a verified GitHub release", Value: "update"},
		{Label: "Roll back application", Description: "Switch to the retained previous version", Value: "rollback"},
		{Label: "Uninstall", Description: "Remove application, lab data, or everything", Value: "uninstall"},
		{Label: "Back", Description: "Return to control center", Value: "back"},
	}
}

func uninstallMenuOptions() []ui.Option {
	return []ui.Option{
		{Label: "Remove application only", Description: "Preserve models, databases, and setup", Value: "application"},
		{Label: "Full uninstall", Description: "Remove app and lab data; keep reinstall choices", Value: "full"},
		{Label: "Remove absolutely everything", Description: "Remove app, data, and reinstall receipt", Value: "absolute"},
		{Label: "Cancel", Description: "Return without removing anything", Value: "cancel"},
	}
}

func (r Runner) application(ctx context.Context) error {
	for {
		choice, err := r.terminal.Select(
			"Local AI Lab application",
			applicationMenuOptions(),
			"update",
		)
		if err != nil {
			return err
		}
		switch choice {
		case "update":
			if err := r.updateApplication(ctx); err != nil {
				return err
			}
		case "rollback":
			if err := r.rollbackApplication(ctx); err != nil {
				return err
			}
		case "uninstall":
			if err := r.uninstallApplication(ctx); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func (r Runner) updateApplication(ctx context.Context) error {
	if r.version == "dev" {
		return r.terminal.Show(
			"Development build",
			"Self-update is disabled for repository builds",
			"Build or install a published release to use verified application updates.",
		)
	}
	checkContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client := distribution.ReleaseClient{}
	release, available, err := client.Latest(
		checkContext,
		r.layout.UpdateCache,
		r.version,
		runtime.GOOS,
		runtime.GOARCH,
		time.Now(),
	)
	if err != nil {
		return err
	}
	if !available {
		return r.terminal.Show(
			"Application is current",
			"No newer GitHub release is available",
			fmt.Sprintf("Installed version  %s", r.version),
		)
	}
	body := fmt.Sprintf(
		"CURRENT   %s\nAVAILABLE %s\nARCHIVE   %s\nSIZE      %s\nSOURCE    %s\n\n"+
			"SHA-256 is verified before extraction. Models and service data are unchanged.",
		r.version,
		release.Version,
		release.Archive.Name,
		formatDataBytes(release.Archive.Size),
		release.PageURL,
	)
	confirmed, err := r.terminal.Review(
		"Update Local AI Lab",
		"Application update only; managed tools remain unchanged",
		body,
		"Download update",
		false,
	)
	if err != nil || !confirmed {
		return err
	}
	err = r.terminal.RunTask(
		ctx,
		"Update Local AI Lab",
		"Verified application update installed",
		func(taskContext context.Context, output io.Writer) error {
			bundle, err := distribution.FetchBundle(
				taskContext,
				distribution.GitHubHTTPClient(5*time.Minute),
				release,
			)
			if err != nil {
				return err
			}
			defer bundle.Remove()
			fmt.Fprintf(output, "verified  %s\n", release.Archive.Name)
			if err := verifyReleaseWithGitHub(taskContext, release, bundle, output); err != nil {
				return err
			}
			installed, err := distribution.NewManager(r.layout).Install(bundle.Root)
			if err != nil {
				return err
			}
			fmt.Fprintf(output, "activate  %s\n", installed.Version)
			return r.recordReceipt(nil, false)
		},
	)
	if err != nil {
		return err
	}
	r.terminal.Success("Restart local-ai-lab to use %s.", release.Version)
	return errApplicationExit
}

func (r Runner) rollbackApplication(ctx context.Context) error {
	manager := distribution.NewManager(r.layout)
	previous, err := manager.PreviousVersion()
	if errors.Is(err, os.ErrNotExist) {
		return r.terminal.Show(
			"No rollback available",
			"No previous application version is retained",
			"Install an application update before using rollback.",
		)
	}
	if err != nil {
		return err
	}
	confirmed, err := r.terminal.Review(
		"Roll back Local AI Lab",
		"Managed tools and lab data remain unchanged",
		"PREVIOUS VERSION\n"+previous,
		"Roll back",
		false,
	)
	if err != nil || !confirmed {
		return err
	}
	version, err := manager.Rollback()
	if err != nil {
		return err
	}
	r.terminal.Success("Application rolled back to %s.", version)
	r.terminal.Info("Restart local-ai-lab to use the rolled back version.")
	return errApplicationExit
}

func (r Runner) uninstallApplication(ctx context.Context) error {
	mode, err := r.terminal.Select("Uninstall Local AI Lab", uninstallMenuOptions(), "cancel")
	if err != nil || mode == "cancel" {
		return err
	}
	installation, loadErr := r.store.Load()
	hasLab := loadErr == nil
	if loadErr != nil && !errors.Is(loadErr, config.ErrNotInstalled) {
		return loadErr
	}
	confirmation := uninstallConfirmation(mode)
	if err := r.previewUninstall(mode, installation, hasLab, confirmation); err != nil {
		return err
	}
	typed, err := r.terminal.Input("Type "+confirmation, "")
	if err != nil {
		return err
	}
	if typed != confirmation {
		return errors.New("uninstall confirmation did not match")
	}
	if hasLab {
		if err := r.executor.Execute(ctx, labruntime.StopPlan(installation)); err != nil {
			return fmt.Errorf("stop services before uninstall: %w", err)
		}
	}
	if mode != "absolute" {
		var currentInstallation *config.Installation
		if hasLab {
			currentInstallation = &installation
		}
		if err := r.recordReceipt(currentInstallation, true); err != nil {
			return err
		}
	}
	if hasLab && mode != "application" {
		plan := app.FullDeletionPlan(r.store.PointerPath(), installation)
		if err := plan.Execute(plan.Confirmation); err != nil {
			return err
		}
		removeBuiltImages(ctx)
	}
	if mode != "application" {
		if err := os.Remove(r.layout.UpdateCache); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove application update cache: %w", err)
		}
	}
	if err := r.removeShellPath(); err != nil {
		return err
	}
	if err := distribution.NewManager(r.layout).RemoveApplication(); err != nil {
		return err
	}
	if mode == "absolute" {
		if err := os.RemoveAll(r.layout.Root); err != nil {
			return fmt.Errorf("remove application data root: %w", err)
		}
	}
	r.terminal.Success("Local AI Lab uninstalled.")
	if mode == "application" && hasLab {
		r.terminal.Info("Lab data is preserved at %s.", installation.DataDir)
	} else if mode == "full" {
		r.terminal.Info("Reinstall choices are preserved at %s.", r.layout.ReinstallReceipt)
	}
	return errApplicationExit
}

func (r Runner) previewUninstall(
	mode string,
	installation config.Installation,
	hasLab bool,
	confirmation string,
) error {
	var body strings.Builder
	fmt.Fprintf(&body, "APPLICATION  %s\nCOMMAND      %s\n", r.layout.AppRoot, r.layout.CommandPath)
	if hasLab {
		if mode == "application" {
			fmt.Fprintf(&body, "LAB DATA     preserved at %s\n", installation.DataDir)
		} else {
			fmt.Fprintf(&body, "LAB DATA     remove %s\n", installation.DataDir)
		}
	}
	switch mode {
	case "application":
		body.WriteString("REINSTALL    choices preserved\n")
	case "full":
		body.WriteString("REINSTALL    choices preserved\n")
		body.WriteString("DEPENDENCIES Docker, drivers, native apps, and shared images retained\n")
	default:
		body.WriteString("REINSTALL    receipt removed\n")
		body.WriteString("DEPENDENCIES Docker, drivers, native apps, and shared images retained\n")
	}
	body.WriteString("\nRunning services are stopped before removal.")
	return r.terminal.Show(
		"Uninstall preview",
		"Nothing is removed before exact text confirmation: "+confirmation,
		body.String(),
	)
}

func uninstallConfirmation(mode string) string {
	switch mode {
	case "application":
		return "UNINSTALL"
	case "full":
		return "UNINSTALL ALL"
	default:
		return "REMOVE EVERYTHING"
	}
}

func (r Runner) recordReceipt(
	installation *config.Installation,
	uninstalled bool,
) error {
	if installation == nil {
		if current, err := r.store.Load(); err == nil {
			installation = &current
		}
	}
	version := r.version
	if current, err := distribution.NewManager(r.layout).CurrentVersion(); err == nil {
		version = current
	}
	existing, _ := distribution.ReadReceipt(r.layout.ReinstallReceipt)
	receipt := buildReceipt(existing, installation, version, uninstalled, time.Now().UTC())
	return distribution.WriteReceipt(r.layout.ReinstallReceipt, receipt)
}

func buildReceipt(
	existing distribution.Receipt,
	installation *config.Installation,
	version string,
	uninstalled bool,
	now time.Time,
) distribution.Receipt {
	receipt := existing
	receipt.Schema = 1
	receipt.LastVersion = version
	if receipt.InstalledAt.IsZero() {
		receipt.InstalledAt = now
	}
	if installation != nil {
		receipt.DataDir = installation.DataDir
		receipt.Platform = installation.Platform
		receipt.Runtime = installation.Runtime
		receipt.Workload = installation.ModelProfile
		receipt.Models = append([]string(nil), installation.Models...)
		receipt.Services = receiptServices(installation.Services)
		receipt.Modules = receiptModules(installation.Modules)
	}
	if uninstalled {
		uninstalledAt := now
		receipt.UninstalledAt = &uninstalledAt
	} else {
		receipt.UninstalledAt = nil
	}
	return receipt
}

func receiptServices(services config.Services) []string {
	selected := make([]string, 0, 4)
	if services.Search {
		selected = append(selected, "search")
	}
	if services.Knowledge {
		selected = append(selected, "knowledge")
	}
	if services.WebUI {
		selected = append(selected, "web-ui")
	}
	if services.Monitoring {
		selected = append(selected, "monitoring")
	}
	return selected
}

func receiptModules(modules config.Modules) []string {
	selected := make([]string, 0, 2)
	if modules.ComfyUI {
		selected = append(selected, "comfyui")
	}
	if modules.OpenCode {
		selected = append(selected, "opencode")
	}
	return selected
}

func (r Runner) removeShellPath() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	manager, err := distribution.NewShellPath(
		homeDir,
		os.Getenv("SHELL"),
		filepath.Dir(r.layout.CommandPath),
	)
	if err != nil {
		return nil
	}
	backup, err := manager.Remove(time.Now())
	if errors.Is(err, distribution.ErrUnsafeShellConfig) {
		r.terminal.Warn("Shell configuration was not modified: %v", err)
		return nil
	}
	if err != nil {
		return err
	}
	if backup != "" {
		r.terminal.Info("Shell config backup: %s", backup)
	}
	return nil
}

func verifyReleaseWithGitHub(
	ctx context.Context,
	release distribution.Release,
	bundle distribution.DownloadedBundle,
	output io.Writer,
) error {
	gh, err := exec.LookPath("gh")
	if err != nil {
		return nil
	}
	repository := "master-bogdan/local-ai-lab"
	commands := [][]string{
		{"release", "verify-asset", release.Version, bundle.ArchivePath, "--repo", repository},
		{"attestation", "verify", bundle.ArchivePath, "--repo", repository},
	}
	for _, arguments := range commands {
		command := exec.CommandContext(ctx, gh, arguments...)
		command.Stdout, command.Stderr = output, output
		if err := command.Run(); err != nil {
			return fmt.Errorf("GitHub verification failed: %w", err)
		}
	}
	fmt.Fprintln(output, "verified  GitHub immutable release and build attestation")
	return nil
}

func removeBuiltImages(ctx context.Context) {
	for _, image := range []string{
		"local-ai-lab/comfyui-cuda:0.29.0",
		"local-ai-lab/comfyui-rocm:0.29.0",
	} {
		_ = exec.CommandContext(ctx, "docker", "image", "rm", image).Run()
	}
}
