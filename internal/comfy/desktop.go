package comfy

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/master-bogdan/local-ai-lab/internal/fileutil"
)

func ConfigureDesktop(homeDir, modelRoot string, now time.Time) (string, error) {
	path := filepath.Join(homeDir, "Library", "Application Support", "ComfyUI", "extra_models_config.yaml")
	document := make(map[string]any)
	existing, err := os.ReadFile(path)
	if err == nil && len(existing) > 0 {
		if err := yaml.Unmarshal(existing, &document); err != nil {
			return "", fmt.Errorf("parse ComfyUI model config: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read ComfyUI model config: %w", err)
	}
	document["local_ai_lab"] = map[string]any{
		"base_path": modelRoot, "is_default": true,
		"checkpoints": "checkpoints", "diffusion_models": "diffusion_models",
		"text_encoders": "text_encoders", "vae": "vae", "loras": "loras",
	}
	payload, err := yaml.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode ComfyUI model config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	backup := ""
	if len(existing) > 0 {
		backup, err = fileutil.Backup(path, existing, ".backup-"+now.Format("20060102-150405"))
		if err != nil {
			return "", fmt.Errorf("backup ComfyUI model config: %w", err)
		}
	}
	if err := fileutil.WriteAtomic(path, payload, 0o600); err != nil {
		return "", err
	}
	return backup, nil
}
