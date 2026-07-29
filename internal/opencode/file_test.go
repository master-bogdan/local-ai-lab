package opencode_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/opencode"
)

func TestConfigPathUsesExistingJSONC(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(homeDir, ".config", "opencode", "opencode.jsonc")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := opencode.ConfigPath(homeDir); got != path {
		t.Fatalf("ConfigPath() = %q, want %q", got, path)
	}
}

func TestWriteWithBackupDoesNotFollowBackupSymlink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte("old config"), 0o600); err != nil {
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

	if _, err := opencode.WriteWithBackup(path, []byte("new config"), now); err == nil {
		t.Fatal("write followed an existing backup symlink")
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "unchanged" {
		t.Fatalf("backup symlink target was overwritten: %q", got)
	}
}
