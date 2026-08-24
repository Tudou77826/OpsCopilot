package updater

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
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

	// —— 下载链路稳定性参数：网络小波动自动重试 + 断点续传 ——
	downloadStallTimeout = 30 * time.Second // 连续无数据多久判定连接卡死（原 60s，自愈更慢）
	maxDownloadAttempts  = 5                // 下载总尝试次数（1 次首发 + 4 次重试）
	retryBaseDelay       = 1 * time.Second  // 重试退避起始值：1s, 2s, 4s, 8s
	maxRetryDelay        = 8 * time.Second
	apiRetryAttempts     = 3 // 版本查询 API 的尝试次数
)

// retrySleep 可在测试中替换，避免真实等待退避时间
var retrySleep = time.Sleep

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
	BytesTotal      int64   `json:"bytesTotal"`
	Percentage      float64 `json:"percentage"`
	SpeedBps        float64 `json:"speedBps"`
	Attempt         int     `json:"attempt,omitempty"` // 当前第几次尝试（≥2 表示发生过断线重连）
	Message         string  `json:"message,omitempty"` // 人读状态提示：重试/续传等
}

// httpStatusError 标记服务端状态码错误，用于判断是否值得重试
type httpStatusError struct {
	Code int
	Body string
}

func (e *httpStatusError) Error() string { return fmt.Sprintf("http %d: %s", e.Code, e.Body) }

// fatalError 本地错误（磁盘/文件系统/参数），重试无意义
type fatalError struct{ err error }

func (e *fatalError) Error() string { return e.err.Error() }
func (e *fatalError) Unwrap() error { return e.err }

// isRetryableErr 判断错误是否值得重试：
// 本地错误不重试；HTTP 4xx（除 429 限流和 416 分片失效）不重试；其余网络类错误重试
func isRetryableErr(err error) bool {
	var fe *fatalError
	if errors.As(err, &fe) {
		return false
	}
	var se *httpStatusError
	if errors.As(err, &se) {
		return se.Code >= 500 || se.Code == http.StatusTooManyRequests || se.Code == http.StatusRequestedRangeNotSatisfiable
	}
	return true
}

func backoffDelay(attempt int) time.Duration {
	d := retryBaseDelay << (attempt - 1) // 1s, 2s, 4s, 8s
	if d > maxRetryDelay {
		d = maxRetryDelay
	}
	return d
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

	// 不清空整个 tempDir：保留 *.part 分片，下载失败后用户再次尝试可续传。
	// 只清理旧的解压产物，避免残留旧文件混入新版本。
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	os.RemoveAll(filepath.Join(tempDir, "extracted"))

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

// FetchReleaseHistory fetches published releases for display in the About panel's
// version log. Returns up to 20 entries (newest first), independent of the update
// flow so users can browse past release notes at any time (issue #44).
func FetchReleaseHistory() ([]ReleaseInfo, error) {
	releases, err := fetchAllReleases()
	if err != nil {
		return nil, err
	}
	// GitHub already returns newest first; cap to a reasonable number for browsing.
	if len(releases) > 20 {
		releases = releases[:20]
	}
	return releases, nil
}

// fetchLatestRelease calls the GitHub API to get the latest release.
// 网络抖动自动重试（apiRetryAttempts 次），检查更新不再因瞬时故障报错。
func fetchLatestRelease() (*ReleaseInfo, error) {
	var release ReleaseInfo
	if err := fetchJSONWithRetry(latestReleaseURL, &release); err != nil {
		return nil, err
	}
	return &release, nil
}

// fetchAllReleases fetches all published releases from GitHub (up to 30 per page).
func fetchAllReleases() ([]ReleaseInfo, error) {
	var releases []ReleaseInfo
	if err := fetchJSONWithRetry(allReleasesURL, &releases); err != nil {
		return nil, err
	}
	return releases, nil
}

// fetchJSONWithRetry GET + JSON 解析，带重试与指数退避。
// 4xx（除 429）属于请求本身的问题，不重试。
func fetchJSONWithRetry(url string, out interface{}) error {
	return fetchJSONWithRetryClient(url, out, newHTTPClient(15*time.Second))
}

func fetchJSONWithRetryClient(url string, out interface{}, client *http.Client) error {
	var lastErr error
	for attempt := 1; attempt <= apiRetryAttempts; attempt++ {
		err := fetchJSONOnce(url, out, client)
		if err == nil {
			return nil
		}
		lastErr = err
		var fe *fatalError
		if errors.As(err, &fe) {
			return err
		}
		var se *httpStatusError
		if errors.As(err, &se) && se.Code < 500 && se.Code != http.StatusTooManyRequests {
			return err
		}
		if attempt == apiRetryAttempts {
			break
		}
		delay := backoffDelay(attempt)
		slog.Warn("updater: api request failed, will retry", "url", url, "attempt", attempt, "delay", delay, "error", err)
		retrySleep(delay)
	}
	return fmt.Errorf("请求 GitHub 失败（已自动重试 %d 次）: %w", apiRetryAttempts-1, lastErr)
}

func fetchJSONOnce(url string, out interface{}, client *http.Client) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return &fatalError{fmt.Errorf("create request: %w", err)}
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("github api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &httpStatusError{Code: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// downloadFile downloads a file with optional progress reporting.
// 网络小波动自动自愈：
//   - 最多 maxDownloadAttempts 次尝试，指数退避（1s/2s/4s/8s）
//   - 断点续传：内容写入 <dest>.part，重试时用 Range 从断点继续；
//     <dest>.part.meta 记录来源 URL，跨次更新 URL 变化时丢弃旧分片，
//     防止把不同版本的包续传到一起
//   - 连接卡死（连续 downloadStallTimeout 无数据）主动断开并重试
func downloadFile(url string, destPath string, progressFn func(DownloadProgress)) error {
	return downloadFileWithClient(url, destPath, progressFn, newHTTPClient(10*time.Minute))
}

func downloadFileWithClient(url string, destPath string, progressFn func(DownloadProgress), client *http.Client) error {
	partPath := destPath + ".part"
	metaPath := partPath + ".meta"

	// 分片来源校验：URL 不一致或无记录 → 丢弃旧分片，从头下载
	if meta, err := os.ReadFile(metaPath); err != nil || strings.TrimSpace(string(meta)) != url {
		os.Remove(partPath)
		if err := os.WriteFile(metaPath, []byte(url), 0644); err != nil {
			return &fatalError{fmt.Errorf("写入分片记录: %w", err)}
		}
	}

	emit := func(p DownloadProgress) {
		if progressFn != nil {
			progressFn(p)
		}
	}

	var lastErr error
	for attempt := 1; attempt <= maxDownloadAttempts; attempt++ {
		err := downloadAttempt(url, partPath, attempt, emit, client)
		if err == nil {
			if err := os.Rename(partPath, destPath); err != nil {
				return &fatalError{fmt.Errorf("移动下载文件: %w", err)}
			}
			os.Remove(metaPath)
			return nil
		}
		lastErr = err
		if !isRetryableErr(err) {
			return err
		}
		if attempt == maxDownloadAttempts {
			break
		}
		delay := backoffDelay(attempt)
		slog.Warn("updater: download attempt failed, will retry",
			"attempt", attempt, "delay", delay, "error", err)
		if off, serr := os.Stat(partPath); serr == nil && off.Size() > 0 {
			emit(DownloadProgress{
				BytesDownloaded: off.Size(),
				Attempt:         attempt + 1,
				Message: fmt.Sprintf("网络波动，%d 秒后自动重试（第 %d/%d 次，已下载部分将续传）",
					int(delay.Seconds()), attempt+1, maxDownloadAttempts),
			})
		} else {
			emit(DownloadProgress{
				Attempt: attempt + 1,
				Message: fmt.Sprintf("网络波动，%d 秒后自动重试（第 %d/%d 次）",
					int(delay.Seconds()), attempt+1, maxDownloadAttempts),
			})
		}
		retrySleep(delay)
	}
	return fmt.Errorf("下载失败（已自动重试 %d 次，仍无法完成）: %w", maxDownloadAttempts-1, lastErr)
}

// downloadAttempt 执行一次下载尝试，支持从已有分片续传。
// 成功时 partPath 已包含完整内容（由调用方改名到位）。
func downloadAttempt(url string, partPath string, attempt int, emit func(DownloadProgress), client *http.Client) error {
	var offset int64
	if st, err := os.Stat(partPath); err == nil {
		offset = st.Size()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return &fatalError{fmt.Errorf("构造下载请求: %w", err)}
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接更新服务器失败: %w", err)
	}
	defer resp.Body.Close()

	var total int64
	resumed := false
	switch {
	case resp.StatusCode == http.StatusPartialContent: // 206：服务器接受续传
		total = parseContentRangeTotal(resp.Header.Get("Content-Range"))
		resumed = offset > 0
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		// 分片异常（比完整文件还大等）：丢弃后重试，下次从头下载
		os.Remove(partPath)
		return &httpStatusError{Code: resp.StatusCode, Body: "分片已失效，丢弃后重新下载"}
	case resp.StatusCode == http.StatusOK:
		if offset > 0 {
			// 服务器不支持 Range（或忽略了）：从头下载
			offset = 0
		}
		total = resp.ContentLength
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return &httpStatusError{Code: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	// 打开分片文件：续传则追加，否则重建
	var f *os.File
	if resumed && offset > 0 {
		f, err = os.OpenFile(partPath, os.O_WRONLY|os.O_APPEND, 0644)
	} else {
		offset = 0
		f, err = os.OpenFile(partPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	}
	if err != nil {
		return &fatalError{fmt.Errorf("打开临时文件: %w", err)}
	}
	defer f.Close()

	if total <= 0 {
		// 长度未知（罕见）：直接复制，出错按可重试处理
		_, err = io.Copy(f, resp.Body)
		if err != nil {
			return fmt.Errorf("下载中断: %w", err)
		}
		return nil
	}

	if resumed && offset > 0 {
		emit(DownloadProgress{
			BytesDownloaded: offset,
			BytesTotal:      total,
			Percentage:      float64(offset) / float64(total) * 100,
			Attempt:         attempt,
			Message:         fmt.Sprintf("断点续传：从 %.1f MB 处继续下载", float64(offset)/(1<<20)),
		})
	}

	// 下载主循环：进度统计 + 卡死检测（连续 stallTimeout 无数据 → 主动断开）
	buf := make([]byte, 32*1024)
	var downloaded int64 = offset
	start := time.Now()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		lastDownloaded := atomic.LoadInt64(&downloaded)
		var stallTicks int
		for {
			select {
			case <-ticker.C:
				cur := atomic.LoadInt64(&downloaded)
				elapsed := time.Since(start).Seconds()
				var speed float64
				if elapsed > 0 {
					speed = float64(cur-offset) / elapsed
				}
				emit(DownloadProgress{
					BytesDownloaded: cur,
					BytesTotal:      total,
					Percentage:      float64(cur) / float64(total) * 100,
					SpeedBps:        speed,
					Attempt:         attempt,
				})
				if cur == lastDownloaded {
					stallTicks++
					if stallTicks >= int(downloadStallTimeout/(500*time.Millisecond)) {
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
				return &fatalError{fmt.Errorf("写入磁盘失败: %w", writeErr)}
			}
			atomic.AddInt64(&downloaded, int64(n))
		}
		// Early exit: if we've received Content-Length bytes, the download
		// is complete.  Some CDN/proxy combos keep the TCP connection open
		// after delivering all data, causing Read to block forever instead
		// of returning io.EOF.
		if atomic.LoadInt64(&downloaded) >= total {
			slog.Info("updater: all bytes received", "downloaded", atomic.LoadInt64(&downloaded), "total", total)
			break
		}
		if readErr != nil {
			close(done)
			if readErr == io.EOF {
				break
			}
			if ctx.Err() != nil {
				return fmt.Errorf("连接停滞 %v 无数据，已自动断开", downloadStallTimeout)
			}
			return fmt.Errorf("网络中断: %w", readErr)
		}
	}

	close(done)

	// 连接对端提前关闭且数据不完整 → 可重试（分片保留供续传）
	if cur := atomic.LoadInt64(&downloaded); cur < total {
		return fmt.Errorf("连接中断（已接收 %d/%d 字节）", cur, total)
	}

	elapsed := time.Since(start).Seconds()
	var speed float64
	if elapsed > 0 {
		speed = float64(total-offset) / elapsed
	}
	emit(DownloadProgress{
		BytesDownloaded: total,
		BytesTotal:      total,
		Percentage:      100,
		SpeedBps:        speed,
		Attempt:         attempt,
	})
	return nil
}

// parseContentRangeTotal 解析 Content-Range（"bytes 100-199/1000"）里的总长度
func parseContentRangeTotal(v string) int64 {
	if v == "" {
		return -1
	}
	i := strings.LastIndex(v, "/")
	if i < 0 || i == len(v)-1 {
		return -1
	}
	n, err := strconv.ParseInt(v[i+1:], 10, 64)
	if err != nil {
		return -1
	}
	return n
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
