package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"

	"github.com/master-bogdan/local-ai-lab/internal/buildinfo"
	"github.com/master-bogdan/local-ai-lab/internal/command"
	"github.com/master-bogdan/local-ai-lab/internal/distribution"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(buildinfo.Version)
		return
	}
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "local-ai-lab-installer: no arguments are supported")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fail(err)
	}
	environment := map[string]string{
		"XDG_BIN_HOME":  os.Getenv("XDG_BIN_HOME"),
		"XDG_DATA_HOME": os.Getenv("XDG_DATA_HOME"),
	}
	terminal := ui.NewTerminal(os.Stdin, os.Stdout)
	terminal.Welcome()
	if err := command.BootstrapApplication(
		ctx,
		distribution.UserLayout(homeDir, runtime.GOOS, environment),
		buildinfo.Version,
		terminal,
	); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "local-ai-lab-installer:", err)
	os.Exit(1)
}
