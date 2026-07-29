package opencode

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/fileutil"
)

func ConfigPath(homeDir string) string {
	configDir := filepath.Join(homeDir, ".config", "opencode")
	if configHome := os.Getenv("XDG_CONFIG_HOME"); configHome != "" {
		configDir = filepath.Join(configHome, "opencode")
	}
	jsonPath := filepath.Join(configDir, "opencode.json")
	if _, err := os.Stat(jsonPath); err == nil {
		return jsonPath
	}
	jsoncPath := filepath.Join(configDir, "opencode.jsonc")
	if _, err := os.Stat(jsoncPath); err == nil {
		return jsoncPath
	}
	return jsonPath
}

func WriteWithBackup(path string, payload []byte, now time.Time) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create OpenCode config directory: %w", err)
	}
	backup := ""
	if existing, err := os.ReadFile(path); err == nil {
		backup, err = fileutil.Backup(path, existing, ".backup-"+now.Format("20060102-150405"))
		if err != nil {
			return "", fmt.Errorf("backup OpenCode config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read OpenCode config: %w", err)
	}
	if err := fileutil.WriteAtomic(path, payload, 0o600); err != nil {
		return "", fmt.Errorf("write OpenCode config: %w", err)
	}
	return backup, nil
}
