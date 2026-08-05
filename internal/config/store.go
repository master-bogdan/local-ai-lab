package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/master-bogdan/local-ai-lab/internal/fileutil"
)

const (
	PointerFile = "installation.json"
	ConfigFile  = "config.json"
)

var ErrNotInstalled = errors.New("local AI lab is not installed")

type Services struct {
	Search     bool `json:"search"`
	Knowledge  bool `json:"knowledge"`
	WebUI      bool `json:"webUI"`
	Monitoring bool `json:"monitoring"`
}

type Installation struct {
	DataDir        string   `json:"dataDir"`
	Platform       string   `json:"platform"`
	Runtime        string   `json:"runtime"`
	GPUVendor      string   `json:"gpuVendor"`
	Experimental   bool     `json:"experimental,omitempty"`
	Models         []string `json:"models"`
	ContextLength  int      `json:"contextLength"`
	EmbeddingModel string   `json:"embeddingModel,omitempty"`
	Services       Services `json:"services"`
	ModelProfile   string   `json:"modelProfile"`
	Workload       string   `json:"workload,omitempty"`
	Modules        Modules  `json:"modules"`
	Secrets        Secrets  `json:"secrets"`
}

type Secrets struct {
	SearXNG string `json:"searxng,omitempty"`
	Grafana string `json:"grafana,omitempty"`
}

type Modules struct {
	ComfyUI     bool     `json:"comfyUI"`
	OpenCode    bool     `json:"openCode"`
	ComfyModels []string `json:"comfyModels,omitempty"`
}

type pointer struct {
	DataDir string `json:"dataDir"`
}

type Store struct {
	rootDir string
}

func NewStore(rootDir string) Store {
	return Store{rootDir: rootDir}
}

func (s Store) PointerPath() string {
	return filepath.Join(s.rootDir, PointerFile)
}

func (s Store) Save(installation Installation) error {
	if err := validateDataDir(installation.DataDir); err != nil {
		return err
	}
	if err := os.MkdirAll(installation.DataDir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if err := writeJSON(filepath.Join(installation.DataDir, ConfigFile), installation); err != nil {
		return fmt.Errorf("write installation config: %w", err)
	}
	if err := os.MkdirAll(s.rootDir, 0o700); err != nil {
		return fmt.Errorf("create application data directory: %w", err)
	}
	if err := writeJSON(s.PointerPath(), pointer{DataDir: installation.DataDir}); err != nil {
		return fmt.Errorf("write installation pointer: %w", err)
	}
	return nil
}

func (s Store) Load() (Installation, error) {
	var location pointer
	if err := readJSON(s.PointerPath(), &location); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Installation{}, ErrNotInstalled
		}
		return Installation{}, fmt.Errorf("read installation pointer: %w", err)
	}
	if err := validateDataDir(location.DataDir); err != nil {
		return Installation{}, fmt.Errorf("validate installation pointer: %w", err)
	}
	var installation Installation
	if err := readJSON(filepath.Join(location.DataDir, ConfigFile), &installation); err != nil {
		return Installation{}, fmt.Errorf("read installation config: %w", err)
	}
	if filepath.Clean(installation.DataDir) != filepath.Clean(location.DataDir) {
		return Installation{}, errors.New("installation config data directory does not match installation pointer")
	}
	return installation, nil
}

func writeJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return fileutil.WriteAtomic(path, payload, 0o600)
}

func readJSON(path string, target any) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, target)
}

func validateDataDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("installation data directory is required")
	}
	if !filepath.IsAbs(path) {
		return errors.New("installation data directory must be absolute")
	}
	return nil
}
