package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/master-bogdan/local-ai-lab/internal/releasepack"
)

func main() {
	var options releasepack.Options
	flag.StringVar(&options.Version, "version", "", "release version, such as v0.1.0")
	flag.StringVar(&options.Commit, "commit", "", "source commit SHA")
	flag.StringVar(&options.OutputDir, "output", "", "new output directory")
	flag.StringVar(&options.ProjectDir, "project", ".", "project root")
	flag.Parse()

	if err := releasepack.Build(context.Background(), options); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}
