package comfy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

type Progress func(asset string, downloaded, total uint64)

type Downloader struct {
	client *http.Client
}

var (
	errInsecureRedirect = errors.New("download redirect must use HTTPS")
	errResponseTooLarge = errors.New("response exceeds catalog size")
)

func NewDownloader(client *http.Client) *Downloader {
	if client == nil {
		return &Downloader{}
	}
	secured := *client
	previousCheck := client.CheckRedirect
	secured.CheckRedirect = func(request *http.Request, previous []*http.Request) error {
		if request.URL == nil || request.URL.Scheme != "https" {
			return errInsecureRedirect
		}
		if previousCheck != nil {
			return previousCheck(request, previous)
		}
		if len(previous) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &Downloader{client: &secured}
}

func (d *Downloader) Install(ctx context.Context, pack Pack, root string, progress Progress) error {
	for _, asset := range pack.Assets {
		if err := validateAsset(asset); err != nil {
			return err
		}
	}
	if d == nil || d.client == nil {
		return errors.New("HTTP client is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create model root: %w", err)
	}
	modelRoot, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("open model root: %w", err)
	}
	defer modelRoot.Close()

	for _, asset := range pack.Assets {
		destination := filepath.FromSlash(asset.Path)
		if matchesChecksum(modelRoot, destination, asset.SHA256) {
			continue
		}
		if err := d.download(ctx, modelRoot, asset, destination, progress); err != nil {
			return fmt.Errorf("download %s: %w", asset.Name, err)
		}
	}
	return nil
}

func (d *Downloader) download(ctx context.Context, root *os.Root, asset Asset, destination string, progress Progress) error {
	if err := root.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	partial := destination + ".part"
	if matchesChecksum(root, partial, asset.SHA256) {
		return root.Rename(partial, destination)
	}
	offset := existingSize(root, partial)
	if asset.SizeBytes > 0 && uint64(offset) >= asset.SizeBytes {
		if err := root.Remove(partial); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("discard invalid completed partial: %w", err)
		}
		offset = 0
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	response, err := d.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.Request == nil || response.Request.URL == nil || response.Request.URL.Scheme != "https" {
		return errors.New("download redirected to an insecure URL")
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("server returned %s", response.Status)
	}
	if response.StatusCode == http.StatusOK {
		offset = 0
	}
	if asset.SizeBytes > 0 && response.ContentLength > int64(asset.SizeBytes)-offset {
		_ = root.Remove(partial)
		return errResponseTooLarge
	}
	if err := appendResponse(root, partial, response.Body, offset, asset, progress); err != nil {
		if errors.Is(err, errResponseTooLarge) {
			_ = root.Remove(partial)
		}
		return err
	}
	if !matchesChecksum(root, partial, asset.SHA256) {
		_ = root.Remove(partial)
		return fmt.Errorf("SHA-256 verification failed")
	}
	return root.Rename(partial, destination)
}

func appendResponse(root *os.Root, path string, source io.Reader, offset int64, asset Asset, progress Progress) error {
	flags := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	file, err := root.OpenFile(path, flags, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	buffer := make([]byte, 1024*1024)
	downloaded := uint64(offset)
	for {
		count, readErr := source.Read(buffer)
		if count > 0 {
			if asset.SizeBytes > 0 && downloaded+uint64(count) > asset.SizeBytes {
				return errResponseTooLarge
			}
			if _, err := file.Write(buffer[:count]); err != nil {
				return err
			}
			downloaded += uint64(count)
			if progress != nil {
				progress(asset.Name, downloaded, asset.SizeBytes)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func validateAsset(asset Asset) error {
	parsed, err := url.Parse(asset.URL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" || parsed.Hostname() != "huggingface.co" {
		return fmt.Errorf("asset %s uses untrusted URL", asset.Name)
	}
	if filepath.IsAbs(asset.Path) || filepath.Clean(asset.Path) != filepath.FromSlash(asset.Path) {
		return fmt.Errorf("asset %s uses unsafe path", asset.Name)
	}
	digest, err := hex.DecodeString(asset.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("asset %s has invalid SHA-256", asset.Name)
	}
	return nil
}

func matchesChecksum(root *os.Root, path, expected string) bool {
	file, err := root.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	return hex.EncodeToString(hash.Sum(nil)) == expected
}

func existingSize(root *os.Root, path string) int64 {
	info, err := root.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
