package distribution

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const ManifestFile = "manifest.json"

var (
	ErrCommandConflict  = errors.New("local-ai-lab command path is not managed by this installation")
	ErrIncompleteBundle = errors.New("application bundle is incomplete")
	versionPattern      = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)
)

type Manifest struct {
	Schema  int    `json:"schema"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

type InstalledBundle struct {
	Version string
	Root    string
}

type Manager struct {
	layout Layout
}

func NewManager(layout Layout) Manager {
	return Manager{layout: layout}
}

func InspectBundle(root string) (Manifest, error) {
	return validateBundle(root)
}

func (m Manager) Install(source string) (InstalledBundle, error) {
	manifest, err := validateBundle(source)
	if err != nil {
		return InstalledBundle{}, err
	}
	if manifest.OS != runtime.GOOS || manifest.Arch != runtime.GOARCH {
		return InstalledBundle{}, fmt.Errorf(
			"bundle platform %s/%s does not match host %s/%s",
			manifest.OS, manifest.Arch, runtime.GOOS, runtime.GOARCH,
		)
	}
	if err := m.checkCommandOwnership(); err != nil {
		return InstalledBundle{}, err
	}
	if err := os.MkdirAll(m.layout.VersionsDir, 0o700); err != nil {
		return InstalledBundle{}, fmt.Errorf("create version directory: %w", err)
	}

	target := filepath.Join(m.layout.VersionsDir, manifest.Version)
	if err := installBundle(source, target); err != nil {
		return InstalledBundle{}, err
	}
	if err := m.activate(target); err != nil {
		return InstalledBundle{}, err
	}
	if err := m.pruneVersions(); err != nil {
		return InstalledBundle{}, err
	}
	return InstalledBundle{Version: manifest.Version, Root: target}, nil
}

func (m Manager) InterruptedInstalls() ([]string, error) {
	entries, err := os.ReadDir(m.layout.VersionsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() && isPartialVersion(entry.Name()) {
			paths = append(paths, filepath.Join(m.layout.VersionsDir, entry.Name()))
		}
	}
	return paths, nil
}

func (m Manager) RemoveInterruptedInstalls() error {
	paths, err := m.InterruptedInstalls()
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove interrupted install %s: %w", path, err)
		}
	}
	return nil
}

func (m Manager) CurrentVersion() (string, error) {
	return m.linkedVersion(m.layout.CurrentLink)
}

func (m Manager) PreviousVersion() (string, error) {
	return m.linkedVersion(m.layout.PreviousLink)
}

func (m Manager) Rollback() (string, error) {
	current, err := m.managedLinkTarget(m.layout.CurrentLink)
	if err != nil {
		return "", fmt.Errorf("resolve current version: %w", err)
	}
	previous, err := m.managedLinkTarget(m.layout.PreviousLink)
	if err != nil {
		return "", fmt.Errorf("resolve previous version: %w", err)
	}
	if _, err := validateBundle(previous); err != nil {
		return "", fmt.Errorf("validate previous version: %w", err)
	}
	if err := replaceSymlink(m.layout.CurrentLink, previous); err != nil {
		return "", fmt.Errorf("activate previous version: %w", err)
	}
	if err := replaceSymlink(m.layout.PreviousLink, current); err != nil {
		return "", fmt.Errorf("retain rolled back version: %w", err)
	}
	return filepath.Base(previous), nil
}

func (m Manager) RemoveApplication() error {
	managed, err := m.commandIsManaged()
	if err != nil {
		return err
	}
	if managed {
		if err := os.Remove(m.layout.CommandPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove command: %w", err)
		}
	}
	if err := os.RemoveAll(m.layout.AppRoot); err != nil {
		return fmt.Errorf("remove application versions: %w", err)
	}
	return nil
}

func (m Manager) activate(target string) error {
	oldCurrent, err := m.managedLinkTarget(m.layout.CurrentLink)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("resolve current version: %w", err)
	}
	if oldCurrent != "" && oldCurrent != target {
		if err := replaceSymlink(m.layout.PreviousLink, oldCurrent); err != nil {
			return fmt.Errorf("retain previous version: %w", err)
		}
	}
	if err := replaceSymlink(m.layout.CurrentLink, target); err != nil {
		return fmt.Errorf("activate version: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.layout.CommandPath), 0o755); err != nil {
		return fmt.Errorf("create command directory: %w", err)
	}
	commandTarget := filepath.Join(m.layout.CurrentLink, CommandName)
	if err := replaceSymlink(m.layout.CommandPath, commandTarget); err != nil {
		return fmt.Errorf("install command: %w", err)
	}
	return nil
}

func (m Manager) pruneVersions() error {
	keep := make(map[string]bool, 2)
	for _, link := range []string{m.layout.CurrentLink, m.layout.PreviousLink} {
		target, err := m.managedLinkTarget(link)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		keep[filepath.Base(target)] = true
	}
	entries, err := os.ReadDir(m.layout.VersionsDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !versionPattern.MatchString(name) || keep[name] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(m.layout.VersionsDir, name)); err != nil {
			return fmt.Errorf("remove retired application version %s: %w", name, err)
		}
	}
	return nil
}

func (m Manager) checkCommandOwnership() error {
	managed, err := m.commandIsManaged()
	if err != nil {
		return err
	}
	if !managed {
		if _, err := os.Lstat(m.layout.CommandPath); err == nil {
			return fmt.Errorf("%w: %s", ErrCommandConflict, m.layout.CommandPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (m Manager) commandIsManaged() (bool, error) {
	target, err := os.Readlink(m.layout.CommandPath)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, fs.ErrInvalid) {
		return false, nil
	}
	if err != nil {
		return false, nil
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(m.layout.CommandPath), target)
	}
	expected := filepath.Join(m.layout.CurrentLink, CommandName)
	return filepath.Clean(target) == filepath.Clean(expected), nil
}

func (m Manager) managedLinkTarget(link string) (string, error) {
	target, err := os.Readlink(link)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(m.layout.VersionsDir, target)
	if err != nil || relative == "." || relative == ".." ||
		filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("version link points outside the managed versions directory")
	}
	return filepath.Clean(target), nil
}

func (m Manager) linkedVersion(link string) (string, error) {
	target, err := m.managedLinkTarget(link)
	if err != nil {
		return "", err
	}
	manifest, err := validateBundle(target)
	if err != nil {
		return "", err
	}
	return manifest.Version, nil
}

func validateBundle(root string) (Manifest, error) {
	payload, err := os.ReadFile(filepath.Join(root, ManifestFile))
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: read manifest: %v", ErrIncompleteBundle, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: parse manifest: %v", ErrIncompleteBundle, err)
	}
	if manifest.Schema != 1 || !versionPattern.MatchString(manifest.Version) ||
		manifest.Commit == "" || manifest.OS == "" || manifest.Arch == "" {
		return Manifest{}, fmt.Errorf("%w: invalid manifest", ErrIncompleteBundle)
	}
	required := []string{
		filepath.Join(root, CommandName),
		filepath.Join(root, "deploy", "compose.yaml"),
	}
	for _, path := range required {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return Manifest{}, fmt.Errorf("%w: missing %s", ErrIncompleteBundle, path)
		}
	}
	return manifest, nil
}

func installBundle(source, target string) error {
	if _, err := os.Stat(target); err == nil {
		_, validateErr := validateBundle(target)
		return validateErr
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(target), "."+filepath.Base(target)+".partial-")
	if err != nil {
		return fmt.Errorf("create bundle staging directory: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := copyBundle(source, staging); err != nil {
		return err
	}
	if _, err := validateBundle(staging); err != nil {
		return err
	}
	if err := os.Rename(staging, target); err != nil {
		return fmt.Errorf("commit application bundle: %w", err)
	}
	return nil
}

func isPartialVersion(name string) bool {
	version, suffix, found := strings.Cut(strings.TrimPrefix(name, "."), ".partial-")
	return found && versionPattern.MatchString(version) && suffix != "" &&
		!strings.ContainsRune(suffix, filepath.Separator)
}

func copyBundle(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == "." {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("bundle contains symlink %s", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.Mkdir(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("bundle contains non-regular file %s", relative)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(source, target string, mode fs.FileMode) (err error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Sync()
}

func replaceSymlink(link, target string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	placeholder, err := os.CreateTemp(filepath.Dir(link), "."+filepath.Base(link)+".tmp-")
	if err != nil {
		return err
	}
	temporary := placeholder.Name()
	if err := placeholder.Close(); err != nil {
		os.Remove(temporary)
		return err
	}
	if err := os.Remove(temporary); err != nil {
		return err
	}
	defer os.Remove(temporary)
	if err := os.Symlink(target, temporary); err != nil {
		return err
	}
	return os.Rename(temporary, link)
}
