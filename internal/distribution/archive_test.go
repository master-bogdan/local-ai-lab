package distribution_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/distribution"
)

func TestFetchBundleVerifiesAndExtractsReleaseArchive(t *testing.T) {
	archive := bundleArchive(t, testBundle(t, "v0.2.0"))
	name := distribution.ArchiveName("v0.2.0", runtime.GOOS, runtime.GOARCH)
	checksums := fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), name)
	server := assetServer(archive, checksums)
	defer server.Close()
	release := distribution.Release{
		Version: "v0.2.0",
		Archive: distribution.ReleaseAsset{
			Name: name, URL: server.URL + "/archive", Size: int64(len(archive)),
		},
		Checksums: distribution.ReleaseAsset{
			Name: "checksums.txt", URL: server.URL + "/checksums", Size: int64(len(checksums)),
		},
	}

	bundle, err := distribution.FetchBundle(context.Background(), server.Client(), release)
	if err != nil {
		t.Fatalf("fetch bundle: %v", err)
	}
	defer bundle.Remove()

	manifest, err := distribution.InspectBundle(bundle.Root)
	if err != nil {
		t.Fatalf("inspect downloaded bundle: %v", err)
	}
	if manifest.Version != "v0.2.0" {
		t.Fatalf("manifest version = %q", manifest.Version)
	}
}

func TestFetchBundleRejectsChecksumMismatch(t *testing.T) {
	archive := bundleArchive(t, testBundle(t, "v0.2.0"))
	name := distribution.ArchiveName("v0.2.0", runtime.GOOS, runtime.GOARCH)
	server := assetServer(archive, strings.Repeat("0", 64)+"  "+name+"\n")
	defer server.Close()
	release := distribution.Release{
		Version:   "v0.2.0",
		Archive:   distribution.ReleaseAsset{Name: name, URL: server.URL + "/archive", Size: int64(len(archive))},
		Checksums: distribution.ReleaseAsset{Name: "checksums.txt", URL: server.URL + "/checksums", Size: 100},
	}

	_, err := distribution.FetchBundle(context.Background(), server.Client(), release)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("fetch error = %v, want checksum failure", err)
	}
}

func TestFetchBundleRejectsPathTraversal(t *testing.T) {
	archive := maliciousArchive(t, "../outside")
	name := distribution.ArchiveName("v0.2.0", runtime.GOOS, runtime.GOARCH)
	checksums := fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), name)
	server := assetServer(archive, checksums)
	defer server.Close()
	release := distribution.Release{
		Version:   "v0.2.0",
		Archive:   distribution.ReleaseAsset{Name: name, URL: server.URL + "/archive", Size: int64(len(archive))},
		Checksums: distribution.ReleaseAsset{Name: "checksums.txt", URL: server.URL + "/checksums", Size: int64(len(checksums))},
	}

	_, err := distribution.FetchBundle(context.Background(), server.Client(), release)
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("fetch error = %v, want unsafe path failure", err)
	}
}

func TestFetchBundleRejectsArchiveSymlink(t *testing.T) {
	archive := maliciousArchive(t, "deploy/link")
	name := distribution.ArchiveName("v0.2.0", runtime.GOOS, runtime.GOARCH)
	checksums := fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), name)
	server := assetServer(archive, checksums)
	defer server.Close()
	release := distribution.Release{
		Version:   "v0.2.0",
		Archive:   distribution.ReleaseAsset{Name: name, URL: server.URL + "/archive", Size: int64(len(archive))},
		Checksums: distribution.ReleaseAsset{Name: "checksums.txt", URL: server.URL + "/checksums", Size: int64(len(checksums))},
	}

	_, err := distribution.FetchBundle(context.Background(), server.Client(), release)
	if err == nil || !strings.Contains(err.Error(), "unsupported archive entry") {
		t.Fatalf("fetch error = %v, want symlink failure", err)
	}
}

func bundleArchive(t *testing.T, root string) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name, err = filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func maliciousArchive(t *testing.T, name string) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	typeflag := byte(tar.TypeReg)
	linkname := ""
	if !strings.Contains(name, "..") {
		typeflag = tar.TypeSymlink
		linkname = "/etc/passwd"
	}
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: name, Typeflag: typeflag, Linkname: linkname, Mode: 0o644, Size: 0,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func assetServer(archive []byte, checksums string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/archive":
			w.Write(archive)
		case "/checksums":
			io.WriteString(w, checksums)
		default:
			http.NotFound(w, request)
		}
	}))
}
