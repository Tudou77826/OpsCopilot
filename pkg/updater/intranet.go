package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"opscopilot/pkg/servicecenter"
)

var ErrChecksum = errors.New("安装包校验失败")

// CheckIntranet never falls back to an external source.
func CheckIntranet(base, current string) (*UpdateStatus, error) {
	return checkIntranet(base, current, fetchJSONWithRetry)
}

func CheckIntranetContext(ctx context.Context, base, current string) (*UpdateStatus, error) {
	return checkIntranet(base, current, func(url string, out any) error { return fetchJSONContext(ctx, url, out) })
}

func checkIntranet(base, current string, fetch func(string, any) error) (*UpdateStatus, error) {
	if _, err := servicecenter.ValidateBase(base); err != nil || base == "" {
		return nil, fmt.Errorf("内网服务地址无效")
	}
	var release ReleaseInfo
	if err := fetch(base+"/api/v1/releases/latest", &release); err != nil {
		return nil, err
	}
	if !servicecenter.ValidVersion(release.TagName) {
		return nil, fmt.Errorf("镜像版本无效")
	}
	for _, a := range release.Assets {
		_, hashErr := hex.DecodeString(a.SHA256)
		if !servicecenter.DownloadURL(base, a.BrowserDownloadURL) || len(a.SHA256) != 64 || hashErr != nil || a.Size <= 0 {
			return nil, fmt.Errorf("镜像附件信息无效")
		}
	}
	status := &UpdateStatus{CurrentVer: current, LatestVer: release.TagName, Source: "intranet", Release: &release, HasUpdate: compareVersions(strings.TrimPrefix(current, "v"), strings.TrimPrefix(release.TagName, "v")) < 0, DownloadURL: selectDownloadURL(release.Assets)}
	if status.DownloadURL == "" {
		return nil, fmt.Errorf("镜像没有可用安装包")
	}
	if status.HasUpdate {
		if history, err := intranetHistory(base, fetch); err == nil {
			status.SkippedVersions, status.Release.Body = cumulativeChangelog(strings.TrimPrefix(current, "v"), status.Release, history)
		}
	}
	return status, nil
}
func IntranetHistory(base string) ([]ReleaseInfo, error) {
	return intranetHistory(base, fetchJSONWithRetry)
}

func IntranetHistoryContext(ctx context.Context, base string) ([]ReleaseInfo, error) {
	return intranetHistory(base, func(url string, out any) error { return fetchJSONContext(ctx, url, out) })
}

func intranetHistory(base string, fetch func(string, any) error) ([]ReleaseInfo, error) {
	var releases []ReleaseInfo
	err := fetch(base+"/api/v1/releases", &releases)
	return releases, err
}
func verifyArtifact(path, expected string, size int64) error {
	if expected == "" {
		return nil
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, f)
	if e != nil {
		return e
	}
	if n != size || hex.EncodeToString(h.Sum(nil)) != strings.ToLower(expected) {
		_ = os.Remove(path)
		return ErrChecksum
	}
	return nil
}
func DownloadVerified(url, dir, sha string, size int64, progress func(DownloadProgress)) (string, error) {
	_, err := hex.DecodeString(sha)
	if len(sha) != 64 || err != nil || size <= 0 {
		return "", fmt.Errorf("缺少安装包校验信息")
	}
	return downloadAndExtract(url, dir, sha, size, progress)
}
