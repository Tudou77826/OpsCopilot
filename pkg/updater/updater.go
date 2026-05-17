package updater

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	owner = "Tudou77826"
	repo  = "OpsCopilot"

	latestReleaseURL = "https://api.github.com/repos/" + owner + "/" + repo + "/releases/latest"
)

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
	HasUpdate    bool         `json:"hasUpdate"`
	CurrentVer   string       `json:"currentVersion"`
	LatestVer    string       `json:"latestVersion"`
	Release      *ReleaseInfo `json:"release,omitempty"`
	DownloadURL  string       `json:"downloadUrl,omitempty"`
	Error        string       `json:"error,omitempty"`
}

// DownloadProgress is emitted as a Wails event during download.
type DownloadProgress struct {
	BytesDownloaded int64   `json:"bytesDownloaded"`
	BytesTotal     int64   `json:"bytesTotal"`
	Percentage     float64 `json:"percentage"`
	SpeedBps       float64 `json:"speedBps"`
}

// Files that should NOT be overwritten during update (user data).
var protectedFiles = map[string]bool{
	"config.json":            true,
	"sessions.json":          true,
	"quick_commands.json":    true,
	"highlight_rules.json":   true,
	"command_whitelist.json": true,
	"file_access.json":       true,
	"mcp.json":               true,
}

// CheckForUpdate queries the GitHub API for the latest release and compares
// versions. currentVersion should be the build-time version string (e.g. "v1.3.4").
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

	// Prefer the standalone exe asset for smaller downloads, fall back to zip.
	downloadURL := ""
	for _, a := range release.Assets {
		if a.Name == "opscopilot.exe" {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		for _, a := range release.Assets {
			if strings.HasSuffix(a.Name, ".zip") {
				downloadURL = a.BrowserDownloadURL
				break
			}
		}
	}

	status := &UpdateStatus{
		HasUpdate:   hasUpdate,
		CurrentVer:  currentVersion,
		LatestVer:   release.TagName,
		Release:     release,
		DownloadURL: downloadURL,
	}
	return status, nil
}

// DownloadAndExtract downloads the release artifact to tempDir and extracts it.
// Returns the path to the extracted directory.
// progressFn is called periodically with download progress (may be nil).
func DownloadAndExtract(downloadURL string, tempDir string, progressFn func(DownloadProgress)) (string, error) {
	isZip := strings.HasSuffix(downloadURL, ".zip")

	// Clean and create temp directory.
	if err := os.RemoveAll(tempDir); err != nil {
		return "", fmt.Errorf("clean temp dir: %w", err)
	}
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	if isZip {
		zipPath := filepath.Join(tempDir, "update.zip")
		if err := downloadFile(downloadURL, zipPath, progressFn); err != nil {
			return "", fmt.Errorf("download: %w", err)
		}
		extractDir := filepath.Join(tempDir, "extracted")
		if err := unzip(zipPath, extractDir); err != nil {
			return "", fmt.Errorf("extract: %w", err)
		}
		return extractDir, nil
	}

	// Standalone exe: download directly.
	exePath := filepath.Join(tempDir, "opscopilot.exe")
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

// ApplyUpdate generates a batch updater script and launches it.
// The calling application should exit immediately after this returns.
func ApplyUpdate(exeDir string, extractedDir string, currentExePath string) error {
	batPath := filepath.Join(os.TempDir(), "opscopilot_update.bat")

	// Determine files to copy from extractedDir.
	// For standalone exe, only the exe itself. For zip, all non-protected files.
	copyCommands := buildCopyCommands(extractedDir, exeDir)

	// Backup current exe as a safety measure.
	backupCmd := fmt.Sprintf(`copy /Y "%s" "%s.bak" >nul 2>nul`, currentExePath, currentExePath)

	script := fmt.Sprintf(`@echo off
echo [UPDATE] OpsCopilot Auto-Update
echo [UPDATE] Waiting for application to exit...
set WAITED=0
:waitloop
tasklist /fi "imagename eq opscopilot.exe" 2>nul | find /i "opscopilot.exe" >nul
if not errorlevel 1 (
    set /a WAITED+=1
    if %%WAITED%% GEQ 30 (
        echo [UPDATE] Timeout waiting for application to exit. Aborting.
        exit /b 1
    )
    timeout /t 1 /nobreak >nul
    goto waitloop
)
echo [UPDATE] Application exited. Applying update...
%s
%s
echo [UPDATE] Update applied. Restarting application...
start "" "%s"
(goto) 2>nul & del "%%~f0"
`, backupCmd, copyCommands, currentExePath)

	if err := os.WriteFile(batPath, []byte(script), 0644); err != nil {
		return fmt.Errorf("write updater script: %w", err)
	}

	slog.Info("updater: launching update script", "script", batPath)
	cmd := exec.Command("cmd", "/C", batPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	return cmd.Start()
}

// buildCopyCommands generates xcopy commands for files that should be replaced.
func buildCopyCommands(srcDir string, dstDir string) string {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		// Fallback: just copy the exe.
		return fmt.Sprintf(`xcopy /Y "%s\opscopilot.exe" "%s\" >nul`, srcDir, dstDir)
	}

	var lines []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if protectedFiles[name] {
			slog.Info("updater: skipping protected file", "file", name)
			continue
		}
		lines = append(lines, fmt.Sprintf(`xcopy /Y "%s\%s" "%s\" >nul`, srcDir, name, dstDir))
	}
	return strings.Join(lines, "\n")
}

// fetchLatestRelease calls the GitHub API to get the latest release.
func fetchLatestRelease() (*ReleaseInfo, error) {
	client := &http.Client{Timeout: 15 * time.Second}
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

// downloadFile downloads a file with optional progress reporting.
func downloadFile(url string, destPath string, progressFn func(DownloadProgress)) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	total := resp.ContentLength
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if progressFn == nil || total <= 0 {
		_, err = io.Copy(f, resp.Body)
		return err
	}

	// Download with progress tracking.
	buf := make([]byte, 32*1024)
	var downloaded int64
	start := time.Now()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(start).Seconds()
				speed := float64(0)
				if elapsed > 0 {
					speed = float64(downloaded) / elapsed
				}
				progressFn(DownloadProgress{
					BytesDownloaded: downloaded,
					BytesTotal:     total,
					Percentage:     float64(downloaded) / float64(total) * 100,
					SpeedBps:       speed,
				})
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
			downloaded += int64(n)
		}
		if readErr != nil {
			close(done)
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}

	close(done)

	// Final progress update.
	progressFn(DownloadProgress{
		BytesDownloaded: downloaded,
		BytesTotal:     total,
		Percentage:     100,
		SpeedBps:       float64(downloaded) / time.Since(start).Seconds(),
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
