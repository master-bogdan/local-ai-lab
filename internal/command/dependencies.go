package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"

	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

type dependencyStep struct {
	description string
	command     string
	args        []string
	stdin       string
}

type dependencyState struct {
	dockerMissing  bool
	dockerNotReady bool
	nvidia         bool
	toolkitMissing bool
	username       string
}

type dependencyExecutor struct {
	terminal *ui.Terminal
	next     planExecutor
}

func (e dependencyExecutor) Execute(ctx context.Context, plan labruntime.Plan) error {
	steps, err := missingDependencySteps(ctx)
	if err != nil {
		return err
	}
	if len(steps) > 0 {
		var body strings.Builder
		for _, step := range steps {
			fmt.Fprintf(&body, "%s\n  %s %s\n\n", step.description, step.command, strings.Join(step.args, " "))
		}
		confirmed, err := e.terminal.Review(
			"System dependency plan", "Commands may request sudo access",
			strings.TrimSpace(body.String()), "Install dependencies", false,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			return errors.New("system dependency installation declined")
		}
		if err := executeDependencySteps(ctx, steps); err != nil {
			return err
		}
		if runtime.GOOS == "linux" && !dockerReady(ctx) {
			return errors.New("docker is installed but this shell lacks access; run `newgrp docker` or log out and back in, then rerun make start")
		}
	}
	return e.next.Execute(ctx, plan)
}

func missingDependencySteps(ctx context.Context) ([]dependencyStep, error) {
	if runtime.GOOS == "darwin" {
		return missingMacDependencies(ctx)
	}
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("unsupported operating system %s", runtime.GOOS)
	}
	return missingLinuxDependencies(ctx)
}

func missingMacDependencies(ctx context.Context) ([]dependencyStep, error) {
	if _, err := exec.LookPath("brew"); err != nil {
		return nil, errors.New("missing Homebrew; install it from https://brew.sh and rerun")
	}
	steps := make([]dependencyStep, 0, 3)
	if _, err := exec.LookPath("docker"); err != nil {
		steps = append(steps, dependencyStep{description: "install Docker Desktop", command: "brew", args: []string{"install", "--cask", "docker"}})
	}
	if _, err := exec.LookPath("ollama"); err != nil {
		steps = append(steps, dependencyStep{description: "install Ollama", command: "brew", args: []string{"install", "--cask", "ollama"}})
	}
	if !dockerReady(ctx) {
		steps = append(steps, dependencyStep{description: "start Docker Desktop", command: "open", args: []string{"-a", "Docker"}})
	}
	return steps, nil
}

func missingLinuxDependencies(ctx context.Context) ([]dependencyStep, error) {
	_, dockerErr := exec.LookPath("docker")
	_, nvidiaErr := exec.LookPath("nvidia-smi")
	_, toolkitErr := exec.LookPath("nvidia-ctk")
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("detect current user: %w", err)
	}
	return linuxDependencySteps(linuxDistro(), dependencyState{
		dockerMissing:  dockerErr != nil || !composeReady(ctx),
		dockerNotReady: !dockerReady(ctx),
		nvidia:         nvidiaErr == nil,
		toolkitMissing: toolkitErr != nil,
		username:       currentUser.Username,
	})
}

func linuxDependencySteps(distro string, state dependencyState) ([]dependencyStep, error) {
	if distro != "fedora" && distro != "ubuntu" && distro != "arch" {
		return nil, fmt.Errorf("automatic dependency installation is unsupported on Linux distribution %q", distro)
	}
	steps := make([]dependencyStep, 0, 12)
	if state.dockerMissing {
		steps = append(steps, dockerInstallSteps(distro)...)
		if state.username != "" {
			steps = append(steps, dependencyStep{
				description: "grant Docker access to " + state.username,
				command:     "sudo", args: []string{"usermod", "-aG", "docker", state.username},
			})
		}
	}
	if state.dockerNotReady {
		steps = append(steps, dependencyStep{description: "start Docker", command: "sudo", args: []string{"systemctl", "start", "docker"}})
	}
	if state.nvidia && state.toolkitMissing {
		steps = append(steps, nvidiaToolkitSteps(distro)...)
	}
	if state.nvidia && (state.toolkitMissing || state.dockerMissing) {
		steps = append(steps,
			dependencyStep{description: "configure NVIDIA Docker runtime", command: "sudo", args: []string{"nvidia-ctk", "runtime", "configure", "--runtime=docker"}},
			dependencyStep{description: "restart Docker", command: "sudo", args: []string{"systemctl", "restart", "docker"}},
		)
	}
	return steps, nil
}

func dockerInstallSteps(distro string) []dependencyStep {
	switch distro {
	case "fedora":
		return []dependencyStep{{description: "install Docker and Compose", command: "sudo", args: []string{"dnf", "install", "-y", "moby-engine", "docker-compose-plugin"}}}
	case "ubuntu":
		return []dependencyStep{
			{description: "refresh Ubuntu packages", command: "sudo", args: []string{"apt-get", "update"}},
			{description: "install Docker and Compose", command: "sudo", args: []string{"apt-get", "install", "-y", "docker.io", "docker-compose-v2"}},
		}
	case "arch":
		return []dependencyStep{{description: "install Docker and Compose", command: "sudo", args: []string{"pacman", "-S", "--needed", "--noconfirm", "docker", "docker-compose"}}}
	default:
		return nil
	}
}

func nvidiaToolkitSteps(distro string) []dependencyStep {
	switch distro {
	case "ubuntu":
		return []dependencyStep{
			{description: "install NVIDIA repository prerequisites", command: "sudo", args: []string{"apt-get", "install", "-y", "ca-certificates", "curl"}},
			{description: "install NVIDIA repository key", command: "sudo", args: []string{"curl", "-fsSL", "https://nvidia.github.io/libnvidia-container/gpgkey", "-o", "/usr/share/keyrings/nvidia-container-toolkit.asc"}},
			{
				description: "configure NVIDIA apt repository", command: "sudo",
				args:  []string{"tee", "/etc/apt/sources.list.d/nvidia-container-toolkit.list"},
				stdin: "deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit.asc] https://nvidia.github.io/libnvidia-container/stable/deb/$(ARCH) /\n",
			},
			{description: "refresh NVIDIA packages", command: "sudo", args: []string{"apt-get", "update"}},
			{description: "install NVIDIA container toolkit", command: "sudo", args: []string{"apt-get", "install", "-y", "nvidia-container-toolkit"}},
		}
	case "fedora":
		return []dependencyStep{
			{description: "install NVIDIA repository prerequisite", command: "sudo", args: []string{"dnf", "install", "-y", "curl"}},
			{description: "configure NVIDIA rpm repository", command: "sudo", args: []string{"curl", "-fsSL", "https://nvidia.github.io/libnvidia-container/stable/rpm/nvidia-container-toolkit.repo", "-o", "/etc/yum.repos.d/nvidia-container-toolkit.repo"}},
			{description: "install NVIDIA container toolkit", command: "sudo", args: []string{"dnf", "install", "-y", "nvidia-container-toolkit"}},
		}
	case "arch":
		return []dependencyStep{{description: "install NVIDIA container toolkit", command: "sudo", args: []string{"pacman", "-S", "--needed", "--noconfirm", "nvidia-container-toolkit"}}}
	default:
		return nil
	}
}

func executeDependencySteps(ctx context.Context, steps []dependencyStep) error {
	for _, step := range steps {
		command := exec.CommandContext(ctx, step.command, step.args...)
		command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
		if step.stdin != "" {
			command.Stdin = strings.NewReader(step.stdin)
			command.Stdout = io.Discard
		}
		if err := command.Run(); err != nil {
			return fmt.Errorf("run %s %s: %w", step.command, strings.Join(step.args, " "), err)
		}
		if step.command == "open" && len(step.args) == 2 && step.args[1] == "Docker" {
			if err := waitForDocker(ctx, 90*time.Second); err != nil {
				return err
			}
		}
	}
	return nil
}

func dockerReady(ctx context.Context) bool {
	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}

func composeReady(ctx context.Context) bool {
	return exec.CommandContext(ctx, "docker", "compose", "version").Run() == nil
}

func waitForDocker(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if dockerReady(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return errors.New("docker did not become ready within 90 seconds")
}

func linuxDistro() string {
	payload, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(payload), "\n") {
		if strings.HasPrefix(line, "ID=") {
			return strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
		}
	}
	return ""
}
