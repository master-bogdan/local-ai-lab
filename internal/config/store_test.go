package config_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/config"
)

func TestStoreSavesAndLoadsInstallationThroughGlobalPointer(t *testing.T) {
	applicationRoot := t.TempDir()
	installation := config.Installation{
		DataDir:  filepath.Join(t.TempDir(), "local-ai-lab"),
		Platform: "linux",
		Workload: "coding",
		Models:   []string{"qwen3.5:9b"},
		Services: config.Services{Search: true, Knowledge: true, WebUI: true},
	}

	store := config.NewStore(applicationRoot)
	if err := store.Save(installation); err != nil {
		t.Fatalf("save installation: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load installation: %v", err)
	}
	if !reflect.DeepEqual(loaded, installation) {
		t.Fatalf("loaded installation differs:\nwant: %#v\n got: %#v", installation, loaded)
	}
	payload, err := os.ReadFile(filepath.Join(installation.DataDir, config.ConfigFile))
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if bytes.Contains(payload, []byte(`"version"`)) || bytes.Contains(payload, []byte(`"lastWorkload"`)) {
		t.Fatalf("config contains migration/history fields:\n%s", payload)
	}
	if !bytes.Contains(payload, []byte(`"workload": "coding"`)) {
		t.Fatalf("config does not persist current workload:\n%s", payload)
	}
}

func TestStoreSaveDoesNotFollowPredictableTemporarySymlink(t *testing.T) {
	applicationRoot := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "local-ai-lab")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dataDir, config.ConfigFile+".tmp")); err != nil {
		t.Fatal(err)
	}

	if err := config.NewStore(applicationRoot).Save(config.Installation{DataDir: dataDir}); err != nil {
		t.Fatalf("save installation: %v", err)
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "unchanged" {
		t.Fatalf("temporary symlink target was overwritten: %q", got)
	}
}

func TestStoreLoadRejectsConfigDataDirectoryMismatch(t *testing.T) {
	applicationRoot := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "local-ai-lab")
	store := config.NewStore(applicationRoot)
	if err := store.Save(config.Installation{DataDir: dataDir}); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(config.Installation{DataDir: filepath.Join(t.TempDir(), "redirected")})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, config.ConfigFile), payload, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Load(); err == nil {
		t.Fatal("loaded config whose data directory differs from installation pointer")
	}
}
