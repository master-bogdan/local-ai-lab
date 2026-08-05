package hardware

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type HostSystem struct{}

func (HostSystem) OS() string {
	return runtime.GOOS
}

func (HostSystem) Arch() string {
	return runtime.GOARCH
}

func (HostSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (HostSystem) Glob(pattern string) (map[string][]byte, error) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err == nil {
			files[path] = content
		}
	}
	return files, nil
}

func (HostSystem) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func existingPath(path string) string {
	for {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}
