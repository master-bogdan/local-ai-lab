package distribution

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	maxArchiveBytes   = 256 * 1024 * 1024
	maxExtractedBytes = 512 * 1024 * 1024
	maxArchiveEntries = 4096
	maxChecksumsBytes = 1024 * 1024
)

type DownloadedBundle struct {
	Root        string
	ArchivePath string
	temporary   string
}

func (b DownloadedBundle) Remove() error {
	return os.RemoveAll(b.temporary)
}

func FetchBundle(
	ctx context.Context,
	client *http.Client,
	release Release,
) (DownloadedBundle, error) {
	if release.Archive.Name != ArchiveName(release.Version, runtime.GOOS, runtime.GOARCH) {
		return DownloadedBundle{}, errors.New("release archive does not match this platform")
	}
	if release.Archive.Size > maxArchiveBytes {
		return DownloadedBundle{}, errors.New("release archive exceeds size limit")
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	temporary, err := os.MkdirTemp("", "local-ai-lab-update-*")
	if err != nil {
		return DownloadedBundle{}, err
	}
	cleanup := func(fetchErr error) (DownloadedBundle, error) {
		os.RemoveAll(temporary)
		return DownloadedBundle{}, fetchErr
	}
	checksums, err := downloadBytes(ctx, client, release.Checksums.URL, maxChecksumsBytes)
	if err != nil {
		return cleanup(fmt.Errorf("download checksums: %w", err))
	}
	expected, err := expectedChecksum(checksums, release.Archive.Name)
	if err != nil {
		return cleanup(err)
	}
	archivePath := filepath.Join(temporary, release.Archive.Name)
	actual, err := downloadFile(ctx, client, release.Archive.URL, archivePath, maxArchiveBytes)
	if err != nil {
		return cleanup(fmt.Errorf("download release archive: %w", err))
	}
	if actual != expected {
		return cleanup(errors.New("release archive checksum does not match checksums.txt"))
	}
	root := filepath.Join(temporary, "bundle")
	if err := os.Mkdir(root, 0o700); err != nil {
		return cleanup(err)
	}
	if err := extractArchive(archivePath, root); err != nil {
		return cleanup(err)
	}
	manifest, err := validateBundle(root)
	if err != nil {
		return cleanup(err)
	}
	if manifest.Version != release.Version {
		return cleanup(fmt.Errorf(
			"bundle version %s does not match release %s",
			manifest.Version,
			release.Version,
		))
	}
	return DownloadedBundle{
		Root: root, ArchivePath: archivePath, temporary: temporary,
	}, nil
}

func downloadBytes(
	ctx context.Context,
	client *http.Client,
	url string,
	limit int64,
) ([]byte, error) {
	response, err := downloadResponse(ctx, client, url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > limit {
		return nil, errors.New("download exceeds size limit")
	}
	return payload, nil
}

func downloadFile(
	ctx context.Context,
	client *http.Client,
	url string,
	target string,
	limit int64,
) (digest [sha256.Size]byte, err error) {
	response, err := downloadResponse(ctx, client, url)
	if err != nil {
		return digest, err
	}
	defer response.Body.Close()
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return digest, err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			os.Remove(target)
		}
	}()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, limit+1))
	if err != nil {
		return digest, err
	}
	if written > limit {
		return digest, errors.New("download exceeds size limit")
	}
	if err := file.Sync(); err != nil {
		return digest, err
	}
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

func downloadResponse(
	ctx context.Context,
	client *http.Client,
	url string,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", CommandName)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("HTTP %s", response.Status)
	}
	return response, nil
}

func expectedChecksum(payload []byte, filename string) ([sha256.Size]byte, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(payload)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != filename {
			continue
		}
		raw, err := hex.DecodeString(fields[0])
		if err != nil || len(raw) != sha256.Size {
			return [sha256.Size]byte{}, errors.New("invalid release archive checksum")
		}
		var digest [sha256.Size]byte
		copy(digest[:], raw)
		return digest, nil
	}
	if err := scanner.Err(); err != nil {
		return [sha256.Size]byte{}, err
	}
	return [sha256.Size]byte{}, fmt.Errorf("checksums.txt has no checksum for %s", filename)
}

func extractArchive(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open release archive: %w", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	var total int64
	for entries := 0; ; entries++ {
		if entries >= maxArchiveEntries {
			return errors.New("release archive contains too many entries")
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read release archive: %w", err)
		}
		relative, err := safeArchivePath(header.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if header.Size < 0 || total+header.Size > maxExtractedBytes {
				return errors.New("release archive exceeds extracted size limit")
			}
			total += header.Size
			if err := extractFile(reader, target, header); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry %s", header.Name)
		}
	}
}

func safeArchivePath(name string) (string, error) {
	cleaned := path.Clean(strings.TrimSpace(name))
	if cleaned == "." || path.IsAbs(cleaned) || cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return cleaned, nil
}

func extractFile(reader io.Reader, target string, header *tar.Header) (err error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(header.Mode) & 0o755
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
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
	if _, err := io.CopyN(file, reader, header.Size); err != nil {
		return err
	}
	return file.Sync()
}
