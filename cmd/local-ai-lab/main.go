package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"

	"github.com/master-bogdan/local-ai-lab/internal/buildinfo"
	"github.com/master-bogdan/local-ai-lab/internal/command"
	"github.com/master-bogdan/local-ai-lab/internal/distribution"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), terminationSignals()...)
	defer stop()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "local-ai-lab:", err)
		os.Exit(1)
	}
	layout := distribution.UserLayout(homeDir, runtime.GOOS, environment())
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "local-ai-lab:", err)
		os.Exit(1)
	}
	appRoot, err := distribution.ResolveApplicationRoot(
		executable,
		os.Getenv("LOCAL_AI_LAB_APP_ROOT"),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "local-ai-lab:", err)
		os.Exit(1)
	}
	terminal := ui.NewTerminal(os.Stdin, os.Stdout)
	if len(os.Args) == 2 && os.Args[1] == "__install" {
		if err := command.InstallApplication(appRoot, layout, terminal); err != nil {
			fmt.Fprintln(os.Stderr, "local-ai-lab:", err)
			os.Exit(1)
		}
		return
	}
	runner := command.NewRunner(
		appRoot,
		executable,
		layout,
		buildinfo.Version,
		buildinfo.Commit,
		terminal,
	)
	if err := runner.Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "local-ai-lab:", err)
		os.Exit(1)
	}
}

func environment() map[string]string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, found := strings.Cut(entry, "=")
		if found {
			values[key] = value
		}
	}
	return values
}
