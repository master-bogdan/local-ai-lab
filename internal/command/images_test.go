package command

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLatestGeneratedImageChoosesNewestRegularSupportedImage(t *testing.T) {
	root := t.TempDir()
	older := filepath.Join(root, "older.png")
	newer := filepath.Join(root, "nested", "newer.jpg")
	if err := os.MkdirAll(filepath.Dir(newer), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{older, newer} {
		if err := os.WriteFile(path, []byte("image"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := os.Chtimes(older, now.Add(-time.Minute), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(newer, filepath.Join(root, "latest.png")); err != nil {
		t.Fatal(err)
	}

	got, err := latestGeneratedImage(root)

	if err != nil {
		t.Fatal(err)
	}
	if got != newer {
		t.Fatalf("latest image = %q, want %q", got, newer)
	}
}

func TestLatestGeneratedImageReportsEmptyOutput(t *testing.T) {
	_, err := latestGeneratedImage(t.TempDir())

	if err == nil {
		t.Fatal("expected empty output error")
	}
}
