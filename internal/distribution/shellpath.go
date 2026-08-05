package distribution

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/fileutil"
)

const (
	PathBlockStart = "# >>> local-ai-lab PATH >>>"
	pathBlockEnd   = "# <<< local-ai-lab PATH <<<"
)

var ErrUnsafeShellConfig = errors.New("shell configuration is unsafe to edit automatically")

type ShellPath struct {
	binDir     string
	configPath string
	line       string
}

func NewShellPath(homeDir, shell, binDir string) (ShellPath, error) {
	if !filepath.IsAbs(binDir) || strings.ContainsAny(binDir, "'\x00\r\n") {
		return ShellPath{}, fmt.Errorf("%w: command directory %q", ErrUnsafeShellConfig, binDir)
	}
	binDir = filepath.Clean(binDir)
	name := filepath.Base(shell)
	manager := ShellPath{binDir: binDir}
	switch name {
	case "zsh":
		manager.configPath = filepath.Join(homeDir, ".zshrc")
		manager.line = `export PATH='` + binDir + `':"$PATH"`
	case "bash":
		manager.configPath = filepath.Join(homeDir, ".bashrc")
		manager.line = `export PATH='` + binDir + `':"$PATH"`
	case "fish":
		manager.configPath = filepath.Join(homeDir, ".config", "fish", "conf.d", "local-ai-lab.fish")
		manager.line = `fish_add_path '` + binDir + `'`
	default:
		return ShellPath{}, fmt.Errorf("unsupported shell %q", name)
	}
	return manager, nil
}

func (s ShellPath) ConfigPath() string {
	return s.configPath
}

func (s ShellPath) Line() string {
	return s.line
}

func (s ShellPath) NeedsChange(pathEnvironment string) bool {
	for _, candidate := range filepath.SplitList(pathEnvironment) {
		if filepath.Clean(candidate) == filepath.Clean(s.binDir) {
			return false
		}
	}
	return true
}

func (s ShellPath) Apply(now time.Time) (string, error) {
	existing, mode, err := s.read()
	if err != nil {
		return "", err
	}
	block := s.block()
	if strings.Contains(string(existing), block) {
		return "", nil
	}
	if hasPathMarker(existing) {
		return "", fmt.Errorf("%w: malformed owned PATH block in %s", ErrUnsafeShellConfig, s.configPath)
	}
	backup, err := backupShellConfig(s.configPath, existing, now)
	if err != nil {
		return "", err
	}
	payload := append([]byte(nil), existing...)
	if len(payload) > 0 && payload[len(payload)-1] != '\n' {
		payload = append(payload, '\n')
	}
	payload = append(payload, block...)
	if err := os.MkdirAll(filepath.Dir(s.configPath), 0o700); err != nil {
		return "", err
	}
	if err := fileutil.WriteAtomic(s.configPath, payload, mode); err != nil {
		return "", fmt.Errorf("update shell configuration: %w", err)
	}
	return backup, nil
}

func (s ShellPath) Remove(now time.Time) (string, error) {
	existing, mode, err := s.read()
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	block := s.block()
	if !strings.Contains(string(existing), block) {
		if hasPathMarker(existing) {
			return "", fmt.Errorf("%w: malformed owned PATH block in %s", ErrUnsafeShellConfig, s.configPath)
		}
		return "", nil
	}
	backup, err := backupShellConfig(s.configPath, existing, now)
	if err != nil {
		return "", err
	}
	payload := []byte(strings.Replace(string(existing), block, "", 1))
	if len(payload) == 0 && filepath.Base(s.configPath) == "local-ai-lab.fish" {
		return backup, os.Remove(s.configPath)
	}
	if err := fileutil.WriteAtomic(s.configPath, payload, mode); err != nil {
		return "", fmt.Errorf("remove shell PATH configuration: %w", err)
	}
	return backup, nil
}

func (s ShellPath) read() ([]byte, os.FileMode, error) {
	info, err := os.Lstat(s.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0o600, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("%w: %s", ErrUnsafeShellConfig, s.configPath)
	}
	payload, err := os.ReadFile(s.configPath)
	return payload, info.Mode().Perm(), err
}

func (s ShellPath) block() string {
	return PathBlockStart + "\n" + s.line + "\n" + pathBlockEnd + "\n"
}

func hasPathMarker(payload []byte) bool {
	content := string(payload)
	return strings.Contains(content, PathBlockStart) || strings.Contains(content, pathBlockEnd)
}

func backupShellConfig(path string, payload []byte, now time.Time) (string, error) {
	if len(payload) == 0 {
		return "", nil
	}
	suffix := ".local-ai-lab.bak." + now.UTC().Format("20060102T150405Z")
	return fileutil.Backup(path, payload, suffix)
}
