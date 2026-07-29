package knowledge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxFileBytes = 1024 * 1024

type IndexStats struct {
	Files  int
	Chunks int
}

type Indexer struct {
	store VectorStore
}

type VectorStore interface {
	Embed(context.Context, string) ([]float64, error)
	Upsert(context.Context, Point) error
}

func NewIndexer(store VectorStore) *Indexer {
	return &Indexer{store: store}
}

func (i *Indexer) Index(ctx context.Context, root string) (IndexStats, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return IndexStats{}, fmt.Errorf("resolve workspace path: %w", err)
	}
	root = absoluteRoot
	paths, err := trackedFiles(ctx, root)
	if err != nil {
		return IndexStats{}, err
	}
	stats := IndexStats{}
	for _, relativePath := range paths {
		content, ok := readableContent(filepath.Join(root, relativePath))
		if !ok || isSecretPath(relativePath) {
			continue
		}
		stats.Files++
		for chunkIndex, chunk := range chunks(string(content), 6000) {
			vector, err := i.store.Embed(ctx, relativePath+"\n"+chunk)
			if err != nil {
				return stats, fmt.Errorf("embed %s: %w", relativePath, err)
			}
			point := Point{
				ID: pointID(root, relativePath, chunkIndex), Path: filepath.Join(root, relativePath),
				Content: chunk, Vector: vector,
			}
			if err := i.store.Upsert(ctx, point); err != nil {
				return stats, fmt.Errorf("index %s: %w", relativePath, err)
			}
			stats.Chunks++
		}
	}
	return stats, nil
}

func trackedFiles(ctx context.Context, root string) ([]string, error) {
	command := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-co", "--exclude-standard", "-z")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("list workspace files with git: %w", err)
	}
	rawPaths := bytes.Split(output, []byte{0})
	paths := make([]string, 0, len(rawPaths))
	for _, path := range rawPaths {
		if len(path) > 0 {
			paths = append(paths, string(path))
		}
	}
	return paths, nil
}

func readableContent(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxFileBytes {
		return nil, false
	}
	content, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
		return nil, false
	}
	return content, true
}

func isSecretPath(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	if name == ".env" || strings.HasPrefix(name, ".env.") {
		return true
	}
	for _, fragment := range []string{"secret", "credential", "id_rsa", "id_ed25519"} {
		if strings.Contains(name, fragment) {
			return true
		}
	}
	return strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".key")
}

func chunks(content string, size int) []string {
	runes := []rune(content)
	if len(runes) <= size {
		return []string{content}
	}
	result := make([]string, 0, len(runes)/size+1)
	for start := 0; start < len(runes); start += size - 400 {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		result = append(result, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return result
}

func pointID(workspace, path string, chunkIndex int) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", workspace, path, chunkIndex)))
	hexDigest := hex.EncodeToString(digest[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexDigest[:8], hexDigest[8:12], hexDigest[12:16], hexDigest[16:20], hexDigest[20:32])
}
