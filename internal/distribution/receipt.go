package distribution

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/fileutil"
)

type Receipt struct {
	Schema        int        `json:"schema"`
	LastVersion   string     `json:"lastVersion"`
	InstalledAt   time.Time  `json:"installedAt"`
	UninstalledAt *time.Time `json:"uninstalledAt,omitempty"`
	DataDir       string     `json:"dataDir,omitempty"`
	Platform      string     `json:"platform,omitempty"`
	Runtime       string     `json:"runtime,omitempty"`
	Workload      string     `json:"workload,omitempty"`
	Models        []string   `json:"models,omitempty"`
	Services      []string   `json:"services,omitempty"`
	Modules       []string   `json:"modules,omitempty"`
}

func WriteReceipt(path string, receipt Receipt) error {
	if receipt.Schema != 1 {
		return errors.New("unsupported reinstall receipt schema")
	}
	payload, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fileutil.WriteAtomic(path, payload, 0o600)
}

func ReadReceipt(path string) (Receipt, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Receipt{}, err
	}
	var receipt Receipt
	if err := json.Unmarshal(payload, &receipt); err != nil {
		return Receipt{}, err
	}
	if receipt.Schema != 1 {
		return Receipt{}, errors.New("unsupported reinstall receipt schema")
	}
	return receipt, nil
}
