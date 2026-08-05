package releasepack

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/distribution"
)

var (
	versionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`)
	commitPattern  = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
)

type Options struct {
	Version    string
	Commit     string
	OutputDir  string
	ProjectDir string
}

type target struct {
	OS   string
	Arch string
}

var releaseTargets = []target{
	{OS: "linux", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
	{OS: "darwin", Arch: "arm64"},
}

func Build(ctx context.Context, options Options) error {
	if !versionPattern.MatchString(options.Version) {
		return fmt.Errorf("invalid release version %q", options.Version)
	}
	if !commitPattern.MatchString(options.Commit) {
		return fmt.Errorf("invalid commit %q", options.Commit)
	}
	if options.ProjectDir == "" {
		options.ProjectDir = "."
	}
	if options.OutputDir == "" {
		return errors.New("release output directory is required")
	}
	if err := os.Mkdir(options.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create release output directory: %w", err)
	}
	checksums := make(map[string]string, len(releaseTargets)*2)
	for _, target := range releaseTargets {
		artifacts, err := buildTarget(ctx, options, target)
		if err != nil {
			return err
		}
		for name, digest := range artifacts {
			checksums[name] = digest
		}
	}
	return writeChecksums(options.OutputDir, checksums)
}

func buildTarget(
	ctx context.Context,
	options Options,
	buildTarget target,
) (map[string]string, error) {
	staging, err := os.MkdirTemp(options.OutputDir, ".bundle-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)
	binary := filepath.Join(staging, distribution.CommandName)
	if err := buildBinary(ctx, options, buildTarget, binary, "./cmd/local-ai-lab"); err != nil {
		return nil, err
	}
	installerName := distribution.InstallerName(buildTarget.OS, buildTarget.Arch)
	installerPath := filepath.Join(options.OutputDir, installerName)
	if err := buildBinary(ctx, options, buildTarget, installerPath, "./cmd/local-ai-lab-installer"); err != nil {
		return nil, err
	}
	if err := copyTree(
		filepath.Join(options.ProjectDir, "deploy"),
		filepath.Join(staging, "deploy"),
	); err != nil {
		return nil, err
	}
	if err := copyRegularFile(
		filepath.Join(options.ProjectDir, "LICENSE"),
		filepath.Join(staging, "LICENSE"),
		0o644,
	); err != nil {
		return nil, err
	}
	manifest := distribution.Manifest{
		Schema: 1, Version: options.Version, Commit: options.Commit,
		OS: buildTarget.OS, Arch: buildTarget.Arch,
	}
	if err := writeManifest(filepath.Join(staging, distribution.ManifestFile), manifest); err != nil {
		return nil, err
	}
	archiveName := distribution.ArchiveName(options.Version, buildTarget.OS, buildTarget.Arch)
	archivePath := filepath.Join(options.OutputDir, archiveName)
	if err := WriteArchive(staging, archivePath); err != nil {
		return nil, err
	}
	archiveDigest, err := fileDigest(archivePath)
	if err != nil {
		return nil, err
	}
	installerDigest, err := fileDigest(installerPath)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		archiveName:   archiveDigest,
		installerName: installerDigest,
	}, nil
}

func buildBinary(
	ctx context.Context,
	options Options,
	buildTarget target,
	outputPath string,
	packagePath string,
) error {
	ldflags := strings.Join([]string{
		"-s", "-w",
		"-X github.com/master-bogdan/local-ai-lab/internal/buildinfo.Version=" + options.Version,
		"-X github.com/master-bogdan/local-ai-lab/internal/buildinfo.Commit=" + options.Commit,
	}, " ")
	command := exec.CommandContext(
		ctx,
		"go", "build",
		"-trimpath",
		"-buildvcs=false",
		"-ldflags", ldflags,
		"-o", outputPath,
		packagePath,
	)
	command.Dir = options.ProjectDir
	command.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+buildTarget.OS,
		"GOARCH="+buildTarget.Arch,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"build %s for %s/%s: %w\n%s",
			packagePath,
			buildTarget.OS,
			buildTarget.Arch,
			err,
			output,
		)
	}
	return nil
}

func WriteArchive(root, target string) (err error) {
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			os.Remove(target)
		}
	}()
	compressed, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		return err
	}
	compressed.Header.ModTime = time.Unix(0, 0)
	archive := tar.NewWriter(compressed)
	paths, err := archivePaths(root)
	if err == nil {
		for _, path := range paths {
			if err = addToArchive(archive, root, path); err != nil {
				break
			}
		}
	}
	if closeErr := archive.Close(); err == nil {
		err = closeErr
	}
	if closeErr := compressed.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = file.Sync()
	}
	return err
}

func archivePaths(root string) ([]string, error) {
	paths := make([]string, 0, 32)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("release bundle contains symlink %s", path)
		}
		paths = append(paths, path)
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

func addToArchive(archive *tar.Writer, root, source string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("release bundle contains unsupported file %s", source)
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	name, err := filepath.Rel(root, source)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(name)
	header.ModTime = time.Unix(0, 0)
	header.AccessTime = time.Time{}
	header.ChangeTime = time.Time{}
	header.Uid, header.Gid = 0, 0
	header.Uname, header.Gname = "", ""
	if err := archive.WriteHeader(header); err != nil || info.IsDir() {
		return err
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(archive, file)
	return err
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("deployment assets contain symlink %s", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("deployment assets contain unsupported file %s", path)
		}
		return copyRegularFile(path, target, info.Mode().Perm())
	})
}

func copyRegularFile(source, target string, mode fs.FileMode) (err error) {
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
		if err != nil {
			os.Remove(target)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Sync()
}

func writeManifest(path string, manifest distribution.Manifest) error {
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o644)
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeChecksums(root string, checksums map[string]string) error {
	names := make([]string, 0, len(checksums))
	for name := range checksums {
		names = append(names, name)
	}
	sort.Strings(names)
	var contents strings.Builder
	for _, name := range names {
		fmt.Fprintf(&contents, "%s  %s\n", checksums[name], name)
	}
	return os.WriteFile(
		filepath.Join(root, "checksums.txt"),
		[]byte(contents.String()),
		0o644,
	)
}
