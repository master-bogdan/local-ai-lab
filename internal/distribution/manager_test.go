package distribution_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/distribution"
)

func TestManagerInstallsVersionedBundleAndCommand(t *testing.T) {
	layout := testLayout(t)
	source := testBundle(t, "v0.1.0")
	manager := distribution.NewManager(layout)

	installed, err := manager.Install(source)
	if err != nil {
		t.Fatalf("install bundle: %v", err)
	}

	wantRoot := filepath.Join(layout.VersionsDir, "v0.1.0")
	if installed.Root != wantRoot || installed.Version != "v0.1.0" {
		t.Fatalf("installed bundle = %#v", installed)
	}
	assertSymlinkTarget(t, layout.CurrentLink, wantRoot)
	assertSymlinkTarget(t, layout.CommandPath, filepath.Join(layout.CurrentLink, distribution.CommandName))
	payload, err := os.ReadFile(filepath.Join(wantRoot, "deploy", "compose.yaml"))
	if err != nil {
		t.Fatalf("read installed deployment asset: %v", err)
	}
	if string(payload) != "services: {}\n" {
		t.Fatalf("deployment asset = %q", payload)
	}
}

func TestManagerUpdateRetainsPreviousVersionForRollback(t *testing.T) {
	layout := testLayout(t)
	manager := distribution.NewManager(layout)
	first := testBundle(t, "v0.1.0")
	second := testBundle(t, "v0.1.1")
	if _, err := manager.Install(first); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Install(second); err != nil {
		t.Fatal(err)
	}

	assertSymlinkTarget(t, layout.CurrentLink, filepath.Join(layout.VersionsDir, "v0.1.1"))
	assertSymlinkTarget(t, layout.PreviousLink, filepath.Join(layout.VersionsDir, "v0.1.0"))

	version, err := manager.Rollback()
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if version != "v0.1.0" {
		t.Fatalf("rollback version = %q", version)
	}
	assertSymlinkTarget(t, layout.CurrentLink, filepath.Join(layout.VersionsDir, "v0.1.0"))
	assertSymlinkTarget(t, layout.PreviousLink, filepath.Join(layout.VersionsDir, "v0.1.1"))
}

func TestManagerUpdateRetainsOnlyCurrentAndPreviousVersions(t *testing.T) {
	layout := testLayout(t)
	manager := distribution.NewManager(layout)
	for _, version := range []string{"v0.1.0", "v0.2.0", "v0.3.0"} {
		if _, err := manager.Install(testBundle(t, version)); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(layout.VersionsDir)
	if err != nil {
		t.Fatal(err)
	}
	var versions []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			versions = append(versions, entry.Name())
		}
	}
	want := []string{"v0.2.0", "v0.3.0"}
	if !reflect.DeepEqual(versions, want) {
		t.Fatalf("retained versions = %#v, want %#v", versions, want)
	}
}

func TestManagerFindsAndRemovesInterruptedInstalls(t *testing.T) {
	layout := testLayout(t)
	if err := os.MkdirAll(layout.VersionsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(layout.VersionsDir, ".v0.2.0.partial-123")
	if err := os.Mkdir(partial, 0o700); err != nil {
		t.Fatal(err)
	}
	manager := distribution.NewManager(layout)

	interrupted, err := manager.InterruptedInstalls()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(interrupted, []string{partial}) {
		t.Fatalf("interrupted installs = %#v", interrupted)
	}
	if err := manager.RemoveInterruptedInstalls(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(partial); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial install still exists: %v", err)
	}
}

func TestManagerRefusesToOverwriteUnmanagedCommand(t *testing.T) {
	layout := testLayout(t)
	if err := os.MkdirAll(filepath.Dir(layout.CommandPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.CommandPath, []byte("unrelated"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := distribution.NewManager(layout).Install(testBundle(t, "v0.1.0"))
	if !errors.Is(err, distribution.ErrCommandConflict) {
		t.Fatalf("install error = %v, want command conflict", err)
	}
	payload, readErr := os.ReadFile(layout.CommandPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(payload) != "unrelated" {
		t.Fatalf("unmanaged command was modified: %q", payload)
	}
}

func TestManagerRejectsBundleSymlinks(t *testing.T) {
	layout := testLayout(t)
	source := testBundle(t, "v0.1.0")
	if err := os.Symlink("/etc/passwd", filepath.Join(source, "deploy", "unexpected")); err != nil {
		t.Fatal(err)
	}

	_, err := distribution.NewManager(layout).Install(source)
	if err == nil {
		t.Fatal("installed a bundle containing a symlink")
	}
}

func TestManagerRemovesApplicationButPreservesLabDataAndReceipt(t *testing.T) {
	layout := testLayout(t)
	manager := distribution.NewManager(layout)
	if _, err := manager.Install(testBundle(t, "v0.1.0")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(layout.DefaultDataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.DefaultDataDir, "model"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.ReinstallReceipt, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := manager.RemoveApplication(); err != nil {
		t.Fatalf("remove application: %v", err)
	}

	if _, err := os.Stat(layout.AppRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("application root still exists: %v", err)
	}
	if _, err := os.Lstat(layout.CommandPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("command still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout.DefaultDataDir, "model")); err != nil {
		t.Fatalf("lab data was removed: %v", err)
	}
	if _, err := os.Stat(layout.ReinstallReceipt); err != nil {
		t.Fatalf("reinstall receipt was removed: %v", err)
	}
}

func testLayout(t *testing.T) distribution.Layout {
	t.Helper()
	home := t.TempDir()
	return distribution.UserLayout(home, runtime.GOOS, nil)
}

func testBundle(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	manifest := distribution.Manifest{
		Schema: 1, Version: version, Commit: "0123456789abcdef",
		OS: runtime.GOOS, Arch: runtime.GOARCH,
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, distribution.ManifestFile), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, distribution.CommandName), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	deploy := filepath.Join(root, "deploy")
	if err := os.MkdirAll(deploy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deploy, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func assertSymlinkTarget(t *testing.T, link, want string) {
	t.Helper()
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("read symlink %s: %v", link, err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		t.Fatal(err)
	}
	want, err = filepath.Abs(want)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(target) != filepath.Clean(want) {
		t.Fatalf("%s -> %s, want %s", link, target, want)
	}
}
