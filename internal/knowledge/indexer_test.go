package knowledge_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/knowledge"
)

type vectorStore struct {
	paths  []string
	points []knowledge.Point
}

func (s *vectorStore) Embed(_ context.Context, text string) ([]float64, error) {
	s.paths = append(s.paths, text)
	return []float64{0.1, 0.2}, nil
}

func (s *vectorStore) Upsert(_ context.Context, point knowledge.Point) error {
	s.points = append(s.points, point)
	return nil
}

func TestIndexerExcludesIgnoredAndSecretFiles(t *testing.T) {
	root := t.TempDir()
	if err := exec.Command("git", "init", "--quiet", root).Run(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ".gitignore", "ignored.txt\n")
	writeFile(t, root, "main.go", "package main\n")
	writeFile(t, root, ".env", "TOKEN=secret\n")
	writeFile(t, root, "ignored.txt", "not indexed\n")
	store := &vectorStore{}

	stats, err := knowledge.NewIndexer(store).Index(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 2 || stats.Chunks != 2 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	for _, embedded := range store.paths {
		if embedded == ".env\nTOKEN=secret\n" || embedded == "ignored.txt\nnot indexed\n" {
			t.Fatalf("excluded content reached embeddings: %q", embedded)
		}
	}
}

func TestIndexerDoesNotFollowWorkspaceSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := exec.Command("git", "init", "--quiet", root).Run(); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(external, []byte("outside workspace secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.txt")
	if err := os.Symlink(external, link); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", root, "add", "linked.txt").Run(); err != nil {
		t.Fatal(err)
	}
	store := &vectorStore{}

	stats, err := knowledge.NewIndexer(store).Index(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 0 || stats.Chunks != 0 || len(store.paths) != 0 {
		t.Fatalf("symlinked content escaped workspace boundary: stats=%#v embedded=%q", stats, store.paths)
	}
}

func TestIndexerNamespacesPointsByWorkspace(t *testing.T) {
	store := &vectorStore{}
	for _, root := range []string{t.TempDir(), t.TempDir()} {
		if err := exec.Command("git", "init", "--quiet", root).Run(); err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, "main.go", "package main\n")
		if _, err := knowledge.NewIndexer(store).Index(context.Background(), root); err != nil {
			t.Fatal(err)
		}
	}

	if len(store.points) != 2 {
		t.Fatalf("expected two indexed points, got %d", len(store.points))
	}
	if store.points[0].ID == store.points[1].ID {
		t.Fatalf("workspaces collided on point ID %q", store.points[0].ID)
	}
	if !filepath.IsAbs(store.points[0].Path) || !filepath.IsAbs(store.points[1].Path) {
		t.Fatalf("knowledge paths do not identify workspaces: %#v", store.points)
	}
}

func TestIndexerUsesStablePointIDWhenContentChanges(t *testing.T) {
	root := t.TempDir()
	if err := exec.Command("git", "init", "--quiet", root).Run(); err != nil {
		t.Fatal(err)
	}
	store := &vectorStore{}
	writeFile(t, root, "main.go", "package first\n")
	if _, err := knowledge.NewIndexer(store).Index(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	firstID := store.points[0].ID

	writeFile(t, root, "main.go", "package second\n")
	if _, err := knowledge.NewIndexer(store).Index(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	secondID := store.points[1].ID

	if firstID != secondID {
		t.Fatalf("edited chunk created stale point IDs: %q != %q", firstID, secondID)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
