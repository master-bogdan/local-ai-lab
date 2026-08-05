package distribution_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/distribution"
)

func TestLayoutUsesLinuxUserDirectories(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "alice")

	layout := distribution.UserLayout(home, "linux", nil)

	root := filepath.Join(home, ".local", "share", "local-ai-lab")
	if layout.Root != root {
		t.Fatalf("root = %q, want %q", layout.Root, root)
	}
	if layout.DefaultDataDir != filepath.Join(root, "data") {
		t.Fatalf("default data directory = %q", layout.DefaultDataDir)
	}
	if layout.CommandPath != filepath.Join(home, ".local", "bin", "local-ai-lab") {
		t.Fatalf("command path = %q", layout.CommandPath)
	}
	if layout.InstallationPointer != filepath.Join(root, "installation.json") {
		t.Fatalf("installation pointer = %q", layout.InstallationPointer)
	}
}

func TestLayoutHonorsXDGDirectories(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "alice")
	environment := map[string]string{
		"XDG_DATA_HOME": filepath.Join(home, "data"),
		"XDG_BIN_HOME":  filepath.Join(home, "commands"),
	}

	layout := distribution.UserLayout(home, "linux", environment)

	if layout.Root != filepath.Join(home, "data", "local-ai-lab") {
		t.Fatalf("root = %q", layout.Root)
	}
	if layout.CommandPath != filepath.Join(home, "commands", "local-ai-lab") {
		t.Fatalf("command path = %q", layout.CommandPath)
	}
}

func TestLayoutIgnoresRelativeXDGDirectories(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "alice")
	layout := distribution.UserLayout(home, "linux", map[string]string{
		"XDG_DATA_HOME": "relative/data",
		"XDG_BIN_HOME":  "relative/bin",
	})

	if layout.Root != filepath.Join(home, ".local", "share", "local-ai-lab") {
		t.Fatalf("root = %q", layout.Root)
	}
	if layout.CommandPath != filepath.Join(home, ".local", "bin", "local-ai-lab") {
		t.Fatalf("command path = %q", layout.CommandPath)
	}
}

func TestLayoutUsesMacApplicationSupport(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "Users", "alice")

	layout := distribution.UserLayout(home, "darwin", nil)

	root := filepath.Join(home, "Library", "Application Support", "local-ai-lab")
	if layout.Root != root {
		t.Fatalf("root = %q, want %q", layout.Root, root)
	}
	if layout.CommandPath != filepath.Join(home, ".local", "bin", "local-ai-lab") {
		t.Fatalf("command path = %q", layout.CommandPath)
	}
}

func TestResolveApplicationRootUsesExplicitDevelopmentRoot(t *testing.T) {
	root := t.TempDir()
	writeBundleMarker(t, root)

	got, err := distribution.ResolveApplicationRoot("/unused/local-ai-lab", root)
	if err != nil {
		t.Fatalf("resolve application root: %v", err)
	}
	if got != root {
		t.Fatalf("application root = %q, want %q", got, root)
	}
}

func TestResolveApplicationRootUsesInstalledBundle(t *testing.T) {
	root := t.TempDir()
	writeBundleMarker(t, root)
	executable := filepath.Join(root, "local-ai-lab")

	got, err := distribution.ResolveApplicationRoot(executable, "")
	if err != nil {
		t.Fatalf("resolve application root: %v", err)
	}
	if got != root {
		t.Fatalf("application root = %q, want %q", got, root)
	}
}

func TestResolveApplicationRootRejectsIncompleteBundle(t *testing.T) {
	_, err := distribution.ResolveApplicationRoot(
		filepath.Join(t.TempDir(), "local-ai-lab"),
		"",
	)
	if err == nil {
		t.Fatal("resolved an application bundle without deployment assets")
	}
}

func writeBundleMarker(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "deploy", "compose.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
