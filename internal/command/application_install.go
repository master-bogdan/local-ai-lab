package command

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/distribution"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

func InstallApplication(
	appRoot string,
	layout distribution.Layout,
	terminal *ui.Terminal,
) error {
	return installApplication(appRoot, layout, terminal, true)
}

func installApplication(
	appRoot string,
	layout distribution.Layout,
	terminal *ui.Terminal,
	review bool,
) error {
	manager := distribution.NewManager(layout)
	continueInstall, err := recoverInterruptedInstall(manager, terminal)
	if err != nil || !continueInstall {
		return err
	}
	manifest, err := distribution.InspectBundle(appRoot)
	if err != nil {
		return err
	}
	if review {
		body := fmt.Sprintf(
			"VERSION      %s\nPLATFORM     %s/%s\nAPPLICATION  %s\nCOMMAND      %s\n\n"+
				"This installs the control center only. No models, containers, or services will start.",
			manifest.Version, manifest.OS, manifest.Arch,
			filepath.Join(layout.VersionsDir, manifest.Version),
			layout.CommandPath,
		)
		confirmed, reviewErr := terminal.Review(
			"Install Local AI Lab",
			"Review application files before continuing",
			body,
			"Install application",
			false,
		)
		if reviewErr != nil || !confirmed {
			return reviewErr
		}
	}
	installed, err := manager.Install(appRoot)
	if err != nil {
		return err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := configureCommandPath(homeDir, layout, terminal); err != nil {
		return err
	}
	receipt := distribution.Receipt{Schema: 1, InstalledAt: time.Now().UTC()}
	if existing, readErr := distribution.ReadReceipt(layout.ReinstallReceipt); readErr == nil {
		receipt = existing
	}
	receipt.Schema = 1
	receipt.LastVersion = installed.Version
	receipt.UninstalledAt = nil
	if err := distribution.WriteReceipt(layout.ReinstallReceipt, receipt); err != nil {
		return fmt.Errorf("write reinstall receipt: %w", err)
	}
	terminal.Success("Local AI Lab %s installed.", installed.Version)
	terminal.Info("Start now: %s", layout.CommandPath)
	terminal.Info("New terminals: local-ai-lab")
	terminal.Info("No models or services were installed or started.")
	return nil
}

func interruptedInstallOptions() []ui.Option {
	return []ui.Option{
		{Label: "Restart installation", Description: "Remove incomplete files and install this verified bundle", Value: "resume"},
		{Label: "Remove incomplete files", Description: "Clean up and exit without installing", Value: "remove"},
		{Label: "Exit", Description: "Leave incomplete files unchanged", Value: "exit"},
	}
}

func recoverInterruptedInstall(
	manager distribution.Manager,
	terminal *ui.Terminal,
) (bool, error) {
	paths, err := manager.InterruptedInstalls()
	if err != nil || len(paths) == 0 {
		return true, err
	}
	choice, err := terminal.Select(
		"Previous installation was interrupted",
		interruptedInstallOptions(),
		"resume",
	)
	if err != nil || choice == "exit" {
		return false, err
	}
	if err := manager.RemoveInterruptedInstalls(); err != nil {
		return false, err
	}
	if choice == "remove" {
		terminal.Success("Incomplete application files removed.")
		return false, nil
	}
	return true, nil
}

func configureCommandPath(
	homeDir string,
	layout distribution.Layout,
	terminal *ui.Terminal,
) error {
	binDir := filepath.Dir(layout.CommandPath)
	shellPath, err := distribution.NewShellPath(
		homeDir,
		os.Getenv("SHELL"),
		binDir,
	)
	if err != nil {
		terminal.Warn("%s is not in PATH.", binDir)
		terminal.Info("Add it to your shell configuration, then open a new terminal.")
		return nil
	}
	if !shellPath.NeedsChange(os.Getenv("PATH")) {
		return nil
	}
	body := fmt.Sprintf(
		"FILE\n%s\n\nCHANGE\n%s\n\nThe existing file is backed up before modification.",
		shellPath.ConfigPath(),
		shellPath.Line(),
	)
	confirmed, err := terminal.Review(
		"Add command to PATH",
		"Optional shell configuration",
		body,
		"Update shell",
		false,
	)
	if err != nil || !confirmed {
		return err
	}
	backup, err := shellPath.Apply(time.Now())
	if err != nil {
		return err
	}
	if backup != "" {
		terminal.Info("Shell config backup: %s", backup)
	}
	return nil
}
