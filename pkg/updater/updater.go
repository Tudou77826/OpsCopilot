package updater

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	owner = "Tudou77826"
	repo  = "OpsCopilot"

	allReleasesURL   = "https://api.github.com/repos/" + owner + "/" + repo + "/releases"
	latestReleaseURL = "https://api.github.com/repos/" + owner + "/" + repo + "/releases/latest"
)

// newHTTPClient creates an HTTP client that respects system proxy settings.
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: systemProxyFunc,
		},
	}
}

// ReleaseInfo represents a GitHub release.
type ReleaseInfo struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

// Asset represents a release asset (downloadable file).
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// UpdateStatus is returned to the frontend as JSON.
type UpdateStatus struct {
	HasUpdate       bool         `json:"hasUpdate"`
	CurrentVer      string       `json:"currentVersion"`
	LatestVer       string       `json:"latestVersion"`
	Release         *ReleaseInfo `json:"release,omitempty"`
	DownloadURL     string       `json:"downloadUrl,omitempty"`
	SkippedVersions []string     `json:"skippedVersions,omitempty"`
	Error           string       `json:"error,omitempty"`
}

// DownloadProgress is emitted as a Wails event during download.
type DownloadProgress struct {
	BytesDownloaded int64   `json:"bytesDownloaded"`
	BytesTotal     int64   `json:"bytesTotal"`
	Percentage     float64 `json:"percentage"`
	SpeedBps       float64 `json:"speedBps"`
}

// Files that should NOT be overwritten during update.
var protectedFiles = map[string]bool{
	// User data
	"config.json":            true,
	"sessions.json":          true,
	"quick_commands.json":    true,
	"highlight_rules.json":   true,
	"command_whitelist.json": true,
	"file_access.json":       true,
	"mcp.json":               true,
	// Independent executables — updated separately
	"mcp-server.exe":  true,
	"OpsFTP.exe":      true,
	"ftpmanager.exe":  true,
}

// CheckForUpdate queries the GitHub API for the latest release and compares
// versions. currentVersion should be the build-time version string (e.g. "v1.3.4").
// If an update is available, it also fetches all releases between the current
// version and the latest, merging their changelogs so the user sees everything new.
func CheckForUpdate(currentVersion string) (*UpdateStatus, error) {
	release, err := fetchLatestRelease()
	if err != nil {
		return &UpdateStatus{
			HasUpdate:  false,
			CurrentVer: currentVersion,
			Error:      err.Error(),
		}, err
	}

	latestVer := strings.TrimPrefix(release.TagName, "v")
	currentVer := strings.TrimPrefix(currentVersion, "v")
	hasUpdate := compareVersions(currentVer, latestVer) < 0

	downloadURL := selectDownloadURL(release.Assets)

	status := &UpdateStatus{
		HasUpdate:   hasUpdate,
		CurrentVer:  currentVersion,
		LatestVer:   release.TagName,
		Release:     release,
		DownloadURL: downloadURL,
	}

	// If there is an update, fetch all intermediate releases to build a
	// cumulative changelog. This is important for users who haven't updated
	// in a while — they'd otherwise only see the latest patch notes.
	if hasUpdate {
		status.SkippedVersions, status.Release.Body = buildCumulativeChangelog(currentVer, release)
	}

	return status, nil
}

// buildCumulativeChangelog fetches all releases and merges the bodies of every
// release newer than currentVer (excluding latestRelease which is already known).
// Returns the list of skipped version tags and the merged changelog body.
func buildCumulativeChangelog(currentVer string, latestRelease *ReleaseInfo) ([]string, string) {
	allReleases, err := fetchAllReleases()
	if err != nil {
		// Fallback: just return the latest release body.
		return nil, latestRelease.Body
	}

	var parts []string
	var skipped []string

	for _, r := range allReleases {
		ver := strings.TrimPrefix(r.TagName, "v")
		// Only include releases strictly newer than current and older than latest.
		if compareVersions(ver, currentVer) > 0 && compareVersions(ver, strings.TrimPrefix(latestRelease.TagName, "v")) < 0 {
			skipped = append(skipped, r.TagName)
		}
	}

	// Build cumulative changelog: latest first, then older versions.
	// Show up to 5 versions of release notes.
	if latestRelease.Body != "" {
		parts = append(parts, fmt.Sprintf("## %s\n\n%s", latestRelease.TagName, latestRelease.Body))
	}

	count := 0
	for _, r := range allReleases {
		if count >= 4 {
			break
		}
		ver := strings.TrimPrefix(r.TagName, "v")
		if compareVersions(ver, currentVer) > 0 && compareVersions(ver, strings.TrimPrefix(latestRelease.TagName, "v")) < 0 {
			if r.Body != "" {
				parts = append(parts, fmt.Sprintf("## %s\n\n%s", r.TagName, r.Body))
				count++
			}
		}
	}

	if len(parts) == 0 {
		return skipped, latestRelease.Body
	}

	return skipped, strings.Join(parts, "\n\n---\n\n")
}

func selectDownloadURL(assets []Asset) string {
	for _, a := range assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".zip") {
			return a.BrowserDownloadURL
		}
	}
	for _, a := range assets {
		if strings.EqualFold(a.Name, "opscopilot.exe") {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// DownloadAndExtract downloads the release artifact to tempDir and extracts it.
// Returns the path to the extracted directory.
// progressFn is called periodically with download progress (may be nil).
func DownloadAndExtract(downloadURL string, tempDir string, progressFn func(DownloadProgress)) (string, error) {
	isZip := strings.HasSuffix(downloadURL, ".zip")
	slog.Info("updater: DownloadAndExtract", "isZip", isZip, "tempDir", tempDir)

	// Clean and create temp directory.
	if err := os.RemoveAll(tempDir); err != nil {
		return "", fmt.Errorf("clean temp dir: %w", err)
	}
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	if isZip {
		zipPath := filepath.Join(tempDir, "update.zip")
		slog.Info("updater: downloading zip...", "zipPath", zipPath)
		if err := downloadFile(downloadURL, zipPath, progressFn); err != nil {
			return "", fmt.Errorf("download: %w", err)
		}
		slog.Info("updater: download complete, extracting...", "zipPath", zipPath)
		extractDir := filepath.Join(tempDir, "extracted")
		if err := unzip(zipPath, extractDir); err != nil {
			return "", fmt.Errorf("extract: %w", err)
		}
		slog.Info("updater: extraction complete", "extractDir", extractDir)
		return extractDir, nil
	}

	// Standalone exe: download directly.
	exePath := filepath.Join(tempDir, "opscopilot.exe")
	slog.Info("updater: downloading exe...", "exePath", exePath)
	if err := downloadFile(downloadURL, exePath, progressFn); err != nil {
		return "", fmt.Errorf("download: %w", err)
	}

	// Verify the exe is non-trivial (at least 1MB).
	info, err := os.Stat(exePath)
	if err != nil {
		return "", fmt.Errorf("stat downloaded exe: %w", err)
	}
	if info.Size() < 1<<20 {
		return "", fmt.Errorf("downloaded exe too small (%d bytes), possibly corrupted", info.Size())
	}

	// Return tempDir as extractDir (exe is directly inside).
	return tempDir, nil
}

// fetchLatestRelease calls the GitHub API to get the latest release.
func fetchLatestRelease() (*ReleaseInfo, error) {
	client := newHTTPClient(15 * time.Second)
	req, err := http.NewRequest("GET", latestReleaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github api returned %d: %s", resp.StatusCode, string(body))
	}

	var release ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &release, nil
}

// fetchAllReleases fetches all published releases from GitHub (up to 30 per page).
func fetchAllReleases() ([]ReleaseInfo, error) {
	client := newHTTPClient(15 * time.Second)
	req, err := http.NewRequest("GET", allReleasesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github api returned %d: %s", resp.StatusCode, string(body))
	}

	var releases []ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return releases, nil
}

// downloadFile downloads a file with optional progress reporting.
// It uses context-based cancellation so that a stalled connection (no data
// for stallTimeout) is detected and aborted quickly instead of blocking for
// the full client timeout.
func downloadFile(url string, destPath string, progressFn func(DownloadProgress)) error {
	const stallTimeout = 60 * time.Second // abort if no data for 60s

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := newHTTPClient(10 * time.Minute)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	slog.Info("updater: sending download request", "url", url)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	total := resp.ContentLength
	slog.Info("updater: download started", "status", resp.StatusCode, "contentLength", total, "dest", destPath)

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if progressFn == nil || total <= 0 {
		_, err = io.Copy(f, resp.Body)
		return err
	}

	// Download with progress tracking and stall detection.
	buf := make([]byte, 32*1024)
	var downloaded int64
	start := time.Now()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		var lastDownloaded int64
		var stallTicks int
		for {
			select {
			case <-ticker.C:
				cur := atomic.LoadInt64(&downloaded)
				elapsed := time.Since(start).Seconds()
				speed := float64(0)
				if elapsed > 0 {
					speed = float64(cur) / elapsed
				}
				progressFn(DownloadProgress{
					BytesDownloaded: cur,
					BytesTotal:     total,
					Percentage:     float64(cur) / float64(total) * 100,
					SpeedBps:       speed,
				})
				// Stall detection: if downloaded hasn't grown, increment counter.
				if cur == lastDownloaded {
					stallTicks++
					if stallTicks >= int(stallTimeout/(500*time.Millisecond)) {
						slog.Warn("updater: download stalled, cancelling", "downloaded", cur, "total", total)
						cancel()
						return
					}
				} else {
					stallTicks = 0
				}
				lastDownloaded = cur
			case <-done:
				return
			}
		}
	}()

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				close(done)
				return writeErr
			}
			atomic.AddInt64(&downloaded, int64(n))
		}
		// Early exit: if we've received Content-Length bytes, the download
		// is complete.  Some CDN/proxy combos keep the TCP connection open
		// after delivering all data, causing Read to block forever instead
		// of returning io.EOF.
		if atomic.LoadInt64(&downloaded) >= total {
			slog.Info("updater: all bytes received, exiting read loop", "downloaded", atomic.LoadInt64(&downloaded), "total", total)
			break
		}
		if readErr != nil {
			close(done)
			if readErr == io.EOF {
				break
			}
			if ctx.Err() != nil {
				return fmt.Errorf("download stalled (no data received for %v)", stallTimeout)
			}
			return readErr
		}
	}

	close(done)

	// Final progress update.
	cur := atomic.LoadInt64(&downloaded)
	slog.Info("updater: download finished", "downloaded", cur, "total", total, "elapsed", time.Since(start).Round(time.Millisecond))
	progressFn(DownloadProgress{
		BytesDownloaded: cur,
		BytesTotal:     total,
		Percentage:     100,
		SpeedBps:       float64(cur) / time.Since(start).Seconds(),
	})

	return nil
}

// unzip extracts a zip archive to destDir.
func unzip(src string, destDir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)

		// Check for zip slip.
		if !strings.HasPrefix(filepath.Clean(fpath), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("zip slip detected: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// compareVersions compares two semver strings (e.g. "1.3.4" vs "1.3.5").
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func compareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	maxLen := len(partsA)
	if len(partsB) > maxLen {
		maxLen = len(partsB)
	}

	for i := 0; i < maxLen; i++ {
		va, vb := 0, 0
		if i < len(partsA) {
			va, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			vb, _ = strconv.Atoi(partsB[i])
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}
