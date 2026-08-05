package distribution

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const CommandName = "local-ai-lab"

type Layout struct {
	Root                string
	AppRoot             string
	VersionsDir         string
	CurrentLink         string
	PreviousLink        string
	DefaultDataDir      string
	InstallationPointer string
	ReinstallReceipt    string
	UpdateCache         string
	CommandPath         string
}

func UserLayout(homeDir, platform string, environment map[string]string) Layout {
	root := applicationDataRoot(homeDir, platform, environment)
	binDir := filepath.Join(homeDir, ".local", "bin")
	if configured := environment["XDG_BIN_HOME"]; validUserPath(configured) && platform != "darwin" {
		binDir = configured
	}
	appRoot := filepath.Join(root, "app")
	return Layout{
		Root:                root,
		AppRoot:             appRoot,
		VersionsDir:         filepath.Join(appRoot, "versions"),
		CurrentLink:         filepath.Join(appRoot, "current"),
		PreviousLink:        filepath.Join(appRoot, "previous"),
		DefaultDataDir:      filepath.Join(root, "data"),
		InstallationPointer: filepath.Join(root, "installation.json"),
		ReinstallReceipt:    filepath.Join(root, "reinstall.json"),
		UpdateCache:         filepath.Join(root, "update.json"),
		CommandPath:         filepath.Join(binDir, CommandName),
	}
}

func ResolveApplicationRoot(executable, developmentRoot string) (string, error) {
	root := developmentRoot
	if root == "" {
		root = filepath.Dir(executable)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	marker := filepath.Join(root, "deploy", "compose.yaml")
	info, err := os.Stat(marker)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("application bundle is missing deploy/compose.yaml")
	}
	return root, nil
}

func applicationDataRoot(homeDir, platform string, environment map[string]string) string {
	if platform == "darwin" {
		return filepath.Join(homeDir, "Library", "Application Support", "local-ai-lab")
	}
	if dataHome := environment["XDG_DATA_HOME"]; validUserPath(dataHome) {
		return filepath.Join(dataHome, "local-ai-lab")
	}
	return filepath.Join(homeDir, ".local", "share", "local-ai-lab")
}

func validUserPath(path string) bool {
	return filepath.IsAbs(path) && !strings.ContainsAny(path, "\x00\r\n")
}
