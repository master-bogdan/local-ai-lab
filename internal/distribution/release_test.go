package distribution_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/distribution"
)

func TestReleaseClientFindsNewerPlatformBundle(t *testing.T) {
	var requests atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		fmt.Fprintf(w, `{
			"tag_name":"v0.2.0",
			"html_url":"https://github.com/master-bogdan/local-ai-lab/releases/tag/v0.2.0",
			"assets":[
				{"name":"checksums.txt","browser_download_url":"%s/checksums","size":128},
				{"name":"local-ai-lab_v0.2.0_%s_%s.tar.gz","browser_download_url":"%s/archive","size":4096}
			]
		}`, server.URL, runtime.GOOS, runtime.GOARCH, server.URL)
	}))
	defer server.Close()
	cachePath := filepath.Join(t.TempDir(), "update.json")
	client := distribution.ReleaseClient{
		HTTPClient: server.Client(),
		LatestURL:  server.URL,
	}

	release, available, err := client.Latest(
		context.Background(),
		cachePath,
		"v0.1.0",
		runtime.GOOS,
		runtime.GOARCH,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("check release: %v", err)
	}
	if !available || release.Version != "v0.2.0" {
		t.Fatalf("release = %#v, available = %t", release, available)
	}
	if release.Archive.Name != distribution.ArchiveName("v0.2.0", runtime.GOOS, runtime.GOARCH) {
		t.Fatalf("archive = %q", release.Archive.Name)
	}

	if _, _, err := client.Latest(
		context.Background(),
		cachePath,
		"v0.1.0",
		runtime.GOOS,
		runtime.GOARCH,
		time.Now().Add(time.Hour),
	); err != nil {
		t.Fatalf("read cached release: %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("release endpoint called %d times, want once", requests.Load())
	}
}

func TestGitHubReleaseBuildsPinnedOfficialAssets(t *testing.T) {
	release, err := distribution.GitHubRelease("v0.2.0", "linux", "arm64")
	if err != nil {
		t.Fatal(err)
	}

	wantRoot := "https://github.com/master-bogdan/local-ai-lab/releases"
	wantArchive := "local-ai-lab_v0.2.0_linux_arm64.tar.gz"
	if release.Version != "v0.2.0" || release.PageURL != wantRoot+"/tag/v0.2.0" {
		t.Fatalf("release = %#v", release)
	}
	if release.Archive.Name != wantArchive ||
		release.Archive.URL != wantRoot+"/download/v0.2.0/"+wantArchive {
		t.Fatalf("archive = %#v", release.Archive)
	}
	if release.Checksums.Name != "checksums.txt" ||
		release.Checksums.URL != wantRoot+"/download/v0.2.0/checksums.txt" {
		t.Fatalf("checksums = %#v", release.Checksums)
	}
}

func TestGitHubReleaseRejectsInvalidVersion(t *testing.T) {
	if _, err := distribution.GitHubRelease("latest", "linux", "amd64"); err == nil {
		t.Fatal("invalid release version accepted")
	}
}

func TestGitHubReleaseRejectsUnsupportedTarget(t *testing.T) {
	if _, err := distribution.GitHubRelease("v0.2.0", "windows", "amd64"); err == nil {
		t.Fatal("unsupported release target accepted")
	}
}

func TestInstallerNameUsesStablePlatformAsset(t *testing.T) {
	got := distribution.InstallerName("darwin", "arm64")
	if got != "local-ai-lab-installer_darwin_arm64" {
		t.Fatalf("installer name = %q", got)
	}
}

func TestReleaseClientDoesNotTreatOlderVersionAsUpdate(t *testing.T) {
	server := releaseServer(t, "v0.1.0")
	defer server.Close()
	client := distribution.ReleaseClient{HTTPClient: server.Client(), LatestURL: server.URL}

	_, available, err := client.Latest(
		context.Background(),
		filepath.Join(t.TempDir(), "update.json"),
		"v0.2.0",
		runtime.GOOS,
		runtime.GOARCH,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if available {
		t.Fatal("older release reported as an update")
	}
}

func TestReleaseClientRejectsMissingPlatformAsset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{
			"tag_name":"v0.2.0",
			"html_url":"https://example.invalid",
			"assets":[{"name":"checksums.txt","browser_download_url":"https://example.invalid/checksums","size":1}]
		}`)
	}))
	defer server.Close()
	client := distribution.ReleaseClient{HTTPClient: server.Client(), LatestURL: server.URL}

	_, _, err := client.Latest(
		context.Background(),
		filepath.Join(t.TempDir(), "update.json"),
		"v0.1.0",
		runtime.GOOS,
		runtime.GOARCH,
		time.Now(),
	)
	if err == nil || !strings.Contains(err.Error(), "platform bundle") {
		t.Fatalf("error = %v", err)
	}
}

func TestReleaseClientUsesFreshCacheWhenOffline(t *testing.T) {
	server := releaseServer(t, "v0.2.0")
	cachePath := filepath.Join(t.TempDir(), "update.json")
	client := distribution.ReleaseClient{HTTPClient: server.Client(), LatestURL: server.URL}
	now := time.Now()
	if _, _, err := client.Latest(
		context.Background(),
		cachePath,
		"v0.1.0",
		runtime.GOOS,
		runtime.GOARCH,
		now,
	); err != nil {
		t.Fatal(err)
	}
	server.Close()

	release, available, err := client.Latest(
		context.Background(),
		cachePath,
		"v0.1.0",
		runtime.GOOS,
		runtime.GOARCH,
		now.Add(time.Hour),
	)
	if err != nil || !available || release.Version != "v0.2.0" {
		t.Fatalf("cached result = %#v, %t, %v", release, available, err)
	}
}

func TestReleaseClientRejectsUntrustedOfficialAssetURL(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := fmt.Sprintf(`{
			"tag_name":"v0.2.0",
			"html_url":"https://github.com/master-bogdan/local-ai-lab/releases/tag/v0.2.0",
			"assets":[
				{"name":"checksums.txt","browser_download_url":"https://evil.invalid/checksums","size":128},
				{"name":%q,"browser_download_url":"https://evil.invalid/archive","size":4096}
			]
		}`, distribution.ArchiveName("v0.2.0", runtime.GOOS, runtime.GOARCH))
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	_, _, err := (distribution.ReleaseClient{HTTPClient: client}).Latest(
		context.Background(),
		filepath.Join(t.TempDir(), "update.json"),
		"v0.1.0",
		runtime.GOOS,
		runtime.GOARCH,
		time.Now(),
	)
	if err == nil || !strings.Contains(err.Error(), "untrusted") {
		t.Fatalf("release error = %v, want untrusted URL", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func releaseServer(t *testing.T, version string) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{
			"tag_name":%q,
			"html_url":"https://example.invalid",
			"assets":[
				{"name":"checksums.txt","browser_download_url":"%s/checksums","size":1},
				{"name":%q,"browser_download_url":"%s/archive","size":1}
			]
		}`, version, server.URL,
			distribution.ArchiveName(version, runtime.GOOS, runtime.GOARCH),
			server.URL,
		)
	}))
	return server
}
