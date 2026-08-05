package distribution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/fileutil"
)

const (
	GitHubLatestReleaseURL = "https://api.github.com/repos/master-bogdan/local-ai-lab/releases/latest"
	releaseCacheLifetime   = 24 * time.Hour
	githubReleaseRoot      = "https://github.com/master-bogdan/local-ai-lab/releases"
)

type ReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type Release struct {
	Version   string
	PageURL   string
	Archive   ReleaseAsset
	Checksums ReleaseAsset
}

type ReleaseClient struct {
	HTTPClient *http.Client
	LatestURL  string
}

type releaseResponse struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []ReleaseAsset `json:"assets"`
}

type releaseCache struct {
	CheckedAt time.Time `json:"checkedAt"`
	Platform  string    `json:"platform"`
	Release   Release   `json:"release"`
}

func ArchiveName(version, platform, architecture string) string {
	return fmt.Sprintf("local-ai-lab_%s_%s_%s.tar.gz", version, platform, architecture)
}

func InstallerName(platform, architecture string) string {
	return fmt.Sprintf("local-ai-lab-installer_%s_%s", platform, architecture)
}

func GitHubRelease(version, platform, architecture string) (Release, error) {
	if !versionPattern.MatchString(version) {
		return Release{}, fmt.Errorf("invalid release version %q", version)
	}
	if !supportedReleaseTarget(platform, architecture) {
		return Release{}, fmt.Errorf("unsupported release target %s/%s", platform, architecture)
	}
	archiveName := ArchiveName(version, platform, architecture)
	assetRoot := githubReleaseRoot + "/download/" + version + "/"
	return Release{
		Version: version,
		PageURL: githubReleaseRoot + "/tag/" + version,
		Archive: ReleaseAsset{
			Name: archiveName,
			URL:  assetRoot + archiveName,
		},
		Checksums: ReleaseAsset{
			Name: "checksums.txt",
			URL:  assetRoot + "checksums.txt",
		},
	}, nil
}

func supportedReleaseTarget(platform, architecture string) bool {
	return platform == "linux" && (architecture == "amd64" || architecture == "arm64") ||
		platform == "darwin" && architecture == "arm64"
}

func (c ReleaseClient) Latest(
	ctx context.Context,
	cachePath string,
	currentVersion string,
	platform string,
	architecture string,
	now time.Time,
) (Release, bool, error) {
	platformKey := platform + "/" + architecture
	if cached, err := readReleaseCache(cachePath); err == nil &&
		cached.Platform == platformKey &&
		now.Sub(cached.CheckedAt) >= 0 &&
		now.Sub(cached.CheckedAt) < releaseCacheLifetime &&
		c.validateRelease(cached.Release) == nil {
		return cached.Release, isNewer(cached.Release.Version, currentVersion), nil
	}
	release, err := c.fetchLatest(ctx, platform, architecture)
	if err != nil {
		return Release{}, false, err
	}
	cache := releaseCache{CheckedAt: now, Platform: platformKey, Release: release}
	if err := writeReleaseCache(cachePath, cache); err != nil {
		return Release{}, false, err
	}
	return release, isNewer(release.Version, currentVersion), nil
}

func (c ReleaseClient) fetchLatest(
	ctx context.Context,
	platform string,
	architecture string,
) (Release, error) {
	url := c.LatestURL
	if url == "" {
		url = GitHubLatestReleaseURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", CommandName)
	client := c.HTTPClient
	if client == nil {
		client = GitHubHTTPClient(10 * time.Second)
	}
	response, err := client.Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("check GitHub release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("check GitHub release: HTTP %s", response.Status)
	}
	var payload releaseResponse
	reader := io.LimitReader(response.Body, 1024*1024)
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return Release{}, fmt.Errorf("decode GitHub release: %w", err)
	}
	if !versionPattern.MatchString(payload.TagName) {
		return Release{}, fmt.Errorf("GitHub returned invalid release version %q", payload.TagName)
	}
	archiveName := ArchiveName(payload.TagName, platform, architecture)
	release := Release{Version: payload.TagName, PageURL: payload.HTMLURL}
	for _, asset := range payload.Assets {
		switch asset.Name {
		case archiveName:
			release.Archive = asset
		case "checksums.txt":
			release.Checksums = asset
		}
	}
	if release.Archive.URL == "" {
		return Release{}, fmt.Errorf("release %s has no platform bundle %s", payload.TagName, archiveName)
	}
	if release.Checksums.URL == "" {
		return Release{}, fmt.Errorf("release %s has no checksums.txt", payload.TagName)
	}
	if err := c.validateRelease(release); err != nil {
		return Release{}, err
	}
	return release, nil
}

func (c ReleaseClient) validateRelease(release Release) error {
	if c.LatestURL != "" {
		return nil
	}
	wantPage := githubReleaseRoot + "/tag/" + release.Version
	wantAssetRoot := githubReleaseRoot + "/download/" + release.Version + "/"
	if release.PageURL != wantPage ||
		release.Archive.URL != wantAssetRoot+release.Archive.Name ||
		release.Checksums.URL != wantAssetRoot+"checksums.txt" {
		return errors.New("GitHub release contains an untrusted URL")
	}
	return nil
}

func GitHubHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many GitHub redirects")
			}
			if request.URL.Scheme != "https" || !trustedGitHubHost(request.URL.Hostname()) {
				return errors.New("GitHub download redirected to an untrusted URL")
			}
			return nil
		},
	}
}

func trustedGitHubHost(host string) bool {
	switch host {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	default:
		return false
	}
}

func readReleaseCache(path string) (releaseCache, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return releaseCache{}, err
	}
	var cache releaseCache
	if err := json.Unmarshal(payload, &cache); err != nil {
		return releaseCache{}, err
	}
	if cache.CheckedAt.IsZero() || !versionPattern.MatchString(cache.Release.Version) {
		return releaseCache{}, errors.New("invalid update cache")
	}
	return cache, nil
}

func writeReleaseCache(path string, cache releaseCache) error {
	payload, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fileutil.WriteAtomic(path, payload, 0o600)
}

func isNewer(candidate, current string) bool {
	candidateVersion, candidateOK := parseVersion(candidate)
	currentVersion, currentOK := parseVersion(current)
	if !candidateOK || !currentOK {
		return false
	}
	for index := range candidateVersion.numbers {
		if candidateVersion.numbers[index] != currentVersion.numbers[index] {
			return candidateVersion.numbers[index] > currentVersion.numbers[index]
		}
	}
	if candidateVersion.prerelease == currentVersion.prerelease {
		return false
	}
	if candidateVersion.prerelease == "" {
		return true
	}
	if currentVersion.prerelease == "" {
		return false
	}
	return candidateVersion.prerelease > currentVersion.prerelease
}

type semanticVersion struct {
	numbers    [3]int
	prerelease string
}

func parseVersion(value string) (semanticVersion, bool) {
	if !versionPattern.MatchString(value) {
		return semanticVersion{}, false
	}
	value = strings.TrimPrefix(value, "v")
	core, prerelease, _ := strings.Cut(value, "-")
	parts := strings.Split(core, ".")
	var version semanticVersion
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			return semanticVersion{}, false
		}
		version.numbers[index] = number
	}
	version.prerelease = prerelease
	return version, true
}
