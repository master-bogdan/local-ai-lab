package distribution_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/distribution"
)

func TestShellPathAddsAndRemovesOwnedZshBlock(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".zshrc")
	original := "export EDITOR=nvim\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := distribution.NewShellPath(home, "/bin/zsh", filepath.Join(home, ".local", "bin"))
	if err != nil {
		t.Fatalf("create shell path manager: %v", err)
	}
	now := time.Date(2026, time.July, 29, 20, 0, 0, 0, time.UTC)

	backup, err := manager.Apply(now)
	if err != nil {
		t.Fatalf("apply path: %v", err)
	}
	if backup == "" {
		t.Fatal("existing shell configuration was not backed up")
	}
	assertFileContents(t, backup, original)
	updated := readFile(t, configPath)
	if !strings.Contains(updated, distribution.PathBlockStart) ||
		!strings.Contains(updated, `export PATH='`+filepath.Join(home, ".local", "bin")+`':"$PATH"`) {
		t.Fatalf("shell configuration missing owned PATH block:\n%s", updated)
	}

	removeBackup, err := manager.Remove(now.Add(time.Minute))
	if err != nil {
		t.Fatalf("remove path: %v", err)
	}
	if removeBackup == "" {
		t.Fatal("updated shell configuration was not backed up before removal")
	}
	assertFileContents(t, configPath, original)
}

func TestShellPathApplyIsIdempotent(t *testing.T) {
	home := t.TempDir()
	manager, err := distribution.NewShellPath(home, "/bin/bash", filepath.Join(home, ".local", "bin"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 29, 20, 0, 0, 0, time.UTC)
	if _, err := manager.Apply(now); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, filepath.Join(home, ".bashrc"))

	backup, err := manager.Apply(now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" {
		t.Fatalf("idempotent apply created backup %q", backup)
	}
	if after := readFile(t, filepath.Join(home, ".bashrc")); after != before {
		t.Fatalf("second apply changed shell configuration:\n%s", after)
	}
}

func TestShellPathRefusesSymlinkedConfiguration(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "zshrc")
	if err := os.WriteFile(target, []byte("unchanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, ".zshrc")); err != nil {
		t.Fatal(err)
	}
	manager, err := distribution.NewShellPath(home, "/bin/zsh", filepath.Join(home, ".local", "bin"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = manager.Apply(time.Now())
	if !errors.Is(err, distribution.ErrUnsafeShellConfig) {
		t.Fatalf("apply error = %v, want unsafe shell config", err)
	}
	assertFileContents(t, target, "unchanged\n")
}

func TestShellPathUsesOwnedFishConfiguration(t *testing.T) {
	home := t.TempDir()
	manager, err := distribution.NewShellPath(home, "/usr/bin/fish", filepath.Join(home, ".local", "bin"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Apply(time.Now()); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(home, ".config", "fish", "conf.d", "local-ai-lab.fish")
	contents := readFile(t, path)
	if !strings.Contains(contents, `fish_add_path '`+filepath.Join(home, ".local", "bin")+`'`) {
		t.Fatalf("fish configuration = %q", contents)
	}
}

func TestShellPathReportsWhenBinDirectoryIsAlreadyAvailable(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	manager, err := distribution.NewShellPath(home, "/bin/zsh", binDir)
	if err != nil {
		t.Fatal(err)
	}

	if manager.NeedsChange("/usr/bin" + string(os.PathListSeparator) + binDir) {
		t.Fatal("PATH change requested for an existing bin directory")
	}
}

func TestShellPathQuotesMetacharactersAsLiteralPath(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, "commands", "$(touch injected)")
	manager, err := distribution.NewShellPath(home, "/bin/zsh", binDir)
	if err != nil {
		t.Fatal(err)
	}
	want := `export PATH='` + binDir + `':"$PATH"`
	if manager.Line() != want {
		t.Fatalf("PATH line = %q, want %q", manager.Line(), want)
	}
}

func TestShellPathRejectsPathThatCannotBeQuotedSafely(t *testing.T) {
	_, err := distribution.NewShellPath(t.TempDir(), "/bin/fish", "/tmp/bad'path")
	if !errors.Is(err, distribution.ErrUnsafeShellConfig) {
		t.Fatalf("shell path error = %v, want unsafe config", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	if got := readFile(t, path); got != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
