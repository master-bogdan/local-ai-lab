package comfy_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/comfy"
)

func TestConfigureDesktopDoesNotFollowBackupSymlink(t *testing.T) {
	homeDir := t.TempDir()
	path := filepath.Join(homeDir, "Library", "Application Support", "ComfyUI", "extra_models_config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("existing: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	backup := path + ".backup-20260716-120000"
	if err := os.Symlink(victim, backup); err != nil {
		t.Fatal(err)
	}

	if _, err := comfy.ConfigureDesktop(homeDir, "/models", now); err == nil {
		t.Fatal("configuration followed an existing backup symlink")
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "unchanged" {
		t.Fatalf("backup symlink target was overwritten: %q", got)
	}
}
