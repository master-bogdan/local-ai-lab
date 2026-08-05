package releasepack_test

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/releasepack"
)

func TestWriteArchiveIsDeterministicAndUsesRelativePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "deploy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "local-ai-lab"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "deploy", "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(t.TempDir(), "first.tar.gz")
	second := filepath.Join(t.TempDir(), "second.tar.gz")

	if err := releasepack.WriteArchive(root, first); err != nil {
		t.Fatal(err)
	}
	if err := releasepack.WriteArchive(root, second); err != nil {
		t.Fatal(err)
	}

	if fileHash(t, first) != fileHash(t, second) {
		t.Fatal("identical bundle produced different archives")
	}
	got := archiveNames(t, first)
	want := []string{"deploy", "deploy/compose.yaml", "local-ai-lab"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("archive entries = %#v, want %#v", got, want)
	}
}

func TestBuildProducesInstallableReleaseAssets(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "dist")
	options := releasepack.Options{
		Version: "v0.1.0", Commit: "0123456789abcdef",
		OutputDir: outputDir, ProjectDir: projectDir,
	}
	if err := releasepack.Build(t.Context(), options); err != nil {
		t.Fatalf("build release: %v", err)
	}

	want := []string{
		"checksums.txt",
		"local-ai-lab-installer_darwin_arm64",
		"local-ai-lab-installer_linux_amd64",
		"local-ai-lab-installer_linux_arm64",
		"local-ai-lab_v0.1.0_darwin_arm64.tar.gz",
		"local-ai-lab_v0.1.0_linux_amd64.tar.gz",
		"local-ai-lab_v0.1.0_linux_arm64.tar.gz",
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("release assets = %#v, want %#v", got, want)
	}
	info, err := os.Stat(filepath.Join(outputDir, "local-ai-lab-installer_linux_amd64"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("installer mode = %o, want 755", info.Mode().Perm())
	}
	installerPath := filepath.Join(
		outputDir,
		"local-ai-lab-installer_"+runtime.GOOS+"_"+runtime.GOARCH,
	)
	version, err := exec.Command(installerPath, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("run native installer: %v\n%s", err, version)
	}
	if strings.TrimSpace(string(version)) != "v0.1.0" {
		t.Fatalf("installer version = %q, want v0.1.0", version)
	}
	if lines := strings.Count(readFile(t, filepath.Join(outputDir, "checksums.txt")), "\n"); lines != 6 {
		t.Fatalf("checksum lines = %d, want 6", lines)
	}
}

func fileHash(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(payload)
}

func archiveNames(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	var names []string
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
	sort.Strings(names)
	return names
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
