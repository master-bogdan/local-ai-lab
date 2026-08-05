package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/distribution"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

func BootstrapApplication(
	ctx context.Context,
	layout distribution.Layout,
	version string,
	terminal *ui.Terminal,
) error {
	if version == "dev" {
		return errors.New("installer was not built for a release")
	}
	release, err := distribution.GitHubRelease(version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	body := fmt.Sprintf(
		"VERSION      %s\nPLATFORM     %s/%s\nAPPLICATION  %s\nCOMMAND      %s\nSOURCE       %s\n\n"+
			"The verified application bundle will be downloaded. No models, containers, or services will start.",
		version,
		runtime.GOOS,
		runtime.GOARCH,
		filepath.Join(layout.VersionsDir, version),
		layout.CommandPath,
		release.PageURL,
	)
	confirmed, err := terminal.Review(
		"Install Local AI Lab",
		"Review download and application paths before continuing",
		body,
		"Download and install",
		false,
	)
	if err != nil || !confirmed {
		return err
	}

	var bundle distribution.DownloadedBundle
	err = terminal.RunTask(
		ctx,
		"Download Local AI Lab",
		"Application bundle verified",
		func(taskContext context.Context, output io.Writer) error {
			downloaded, downloadErr := distribution.FetchBundle(
				taskContext,
				distribution.GitHubHTTPClient(5*time.Minute),
				release,
			)
			if downloadErr != nil {
				return downloadErr
			}
			if verifyErr := verifyReleaseWithGitHub(taskContext, release, downloaded, output); verifyErr != nil {
				downloaded.Remove()
				return verifyErr
			}
			fmt.Fprintf(output, "verified  %s\n", release.Archive.Name)
			bundle = downloaded
			return nil
		},
	)
	if err != nil {
		return err
	}
	defer bundle.Remove()
	return installApplication(bundle.Root, layout, terminal, false)
}
