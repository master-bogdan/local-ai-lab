package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/master-bogdan/local-ai-lab/internal/config"
)

type DeletionCategory string

const (
	DeleteModels      DeletionCategory = "models"
	DeleteCache       DeletionCategory = "cache"
	DeleteIndexes     DeletionCategory = "indexes"
	DeleteServiceData DeletionCategory = "service-data"
)

type DeletionPlan struct {
	Paths        []string
	Confirmation string
	DataRoot     string
	RepoPointer  string
	Categories   []DeletionCategory
}

type DeletionTarget struct {
	Path      string
	SizeBytes int64
}

func FullDeletionPlan(repoDir string, installation config.Installation) DeletionPlan {
	pointerPath := filepath.Join(repoDir, config.PointerFile)
	return DeletionPlan{
		Paths:        []string{installation.DataDir, pointerPath},
		Confirmation: "DELETE",
		DataRoot:     installation.DataDir,
		RepoPointer:  pointerPath,
	}
}

func PartialDeletionPlan(installation config.Installation, categories []DeletionCategory) DeletionPlan {
	paths := make([]string, 0, len(categories))
	for _, category := range categories {
		switch category {
		case DeleteModels:
			paths = append(paths, filepath.Join(installation.DataDir, "models"))
		case DeleteCache:
			paths = append(paths, filepath.Join(installation.DataDir, "cache"))
		case DeleteIndexes:
			paths = append(paths, filepath.Join(installation.DataDir, "qdrant"))
		case DeleteServiceData:
			paths = append(paths, filepath.Join(installation.DataDir, "services"))
		}
	}
	return DeletionPlan{
		Paths: paths, Confirmation: "DELETE SELECTED", DataRoot: installation.DataDir,
		Categories: append([]DeletionCategory(nil), categories...),
	}
}

func (p DeletionPlan) Includes(category DeletionCategory) bool {
	for _, selected := range p.Categories {
		if selected == category {
			return true
		}
	}
	return false
}

func (p DeletionPlan) Preview() ([]DeletionTarget, error) {
	targets := make([]DeletionTarget, 0, len(p.Paths))
	for _, path := range p.Paths {
		size, err := DirectorySize(path)
		if err != nil {
			return nil, fmt.Errorf("measure %s: %w", path, err)
		}
		targets = append(targets, DeletionTarget{Path: path, SizeBytes: size})
	}
	return targets, nil
}

func (p DeletionPlan) Execute(confirmation string) error {
	if confirmation != p.Confirmation {
		return errors.New("deletion confirmation did not match")
	}
	for _, path := range p.Paths {
		if path != p.RepoPointer && !isSafeDeletionTarget(p.DataRoot, path) {
			return fmt.Errorf("refusing unsafe deletion path %q", path)
		}
		if path == p.RepoPointer && filepath.Base(path) != config.PointerFile {
			return fmt.Errorf("refusing unsafe repository pointer %q", path)
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("delete %s: %w", path, err)
		}
	}
	return nil
}

func DirectorySize(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err == nil {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func isSafeDeletionTarget(dataRoot, target string) bool {
	if !isSafeDataRoot(dataRoot) {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(dataRoot), filepath.Clean(target))
	return err == nil && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func isSafeDataRoot(path string) bool {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return false
	}

	allowedRoots := []string{"/data", "/mnt"}
	if homeDir, err := os.UserHomeDir(); err == nil {
		allowedRoots = append(allowedRoots, homeDir)
	}
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		allowedRoots = append(allowedRoots, dataHome)
	}
	for _, root := range allowedRoots {
		if isPathBelow(root, cleaned) {
			return true
		}
	}
	return false
}

func isPathBelow(root, path string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), path)
	return err == nil &&
		relative != "." &&
		relative != ".." &&
		!filepath.IsAbs(relative) &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
