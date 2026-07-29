package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/master-bogdan/local-ai-lab/internal/command"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), terminationSignals()...)
	defer stop()

	repoDir, err := findRepoDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "localai:", err)
		os.Exit(1)
	}
	terminal := ui.NewTerminal(os.Stdin, os.Stdout)
	runner := command.NewRunner(repoDir, terminal)
	if err := runner.Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "localai:", err)
		os.Exit(1)
	}
}

func findRepoDir() (string, error) {
	if configured := os.Getenv("LOCAL_AI_LAB_REPO"); configured != "" {
		return filepath.Abs(configured)
	}
	workingDir, err := os.Getwd()
	if err == nil && fileExists(filepath.Join(workingDir, "go.mod")) {
		return workingDir, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	candidate := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", ".."))
	if !fileExists(filepath.Join(candidate, "go.mod")) {
		return "", fmt.Errorf("run from local-ai-lab repository")
	}
	return candidate, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
