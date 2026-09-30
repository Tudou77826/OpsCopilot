package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"opscopilot/pkg/servicecenter"
)

const checkBudget = 10 * time.Second
const githubBudgetWithFallback = 5 * time.Second

// CheckPreferred checks GitHub first, then the configured intranet service if
// GitHub fails. Both attempts share a ten-second deadline.
func CheckPreferred(parent context.Context, current, base string, stage func(string)) (*UpdateStatus, error) {
	return checkPreferred(parent, current, base, stage, CheckGitHubContext, CheckIntranetContext)
}

func checkPreferred(parent context.Context, current, base string, stage func(string), github func(context.Context, string) (*UpdateStatus, error), intranet func(context.Context, string, string) (*UpdateStatus, error)) (*UpdateStatus, error) {
	ctx, cancel := context.WithTimeout(parent, checkBudget)
	defer cancel()
	githubBudget := checkBudget
	if base != "" {
		githubBudget = githubBudgetWithFallback
	}
	if stage != nil {
		stage("github")
	}
	githubCtx, stopGithub := context.WithTimeout(ctx, githubBudget)
	status, githubErr := github(githubCtx, current)
	stopGithub()
	if githubErr == nil {
		return status, nil
	}
	if base == "" {
		return nil, githubErr
	}
	if stage != nil {
		stage("intranet")
	}
	status, intranetErr := intranet(ctx, base, current)
	if intranetErr == nil {
		return status, nil
	}
	return nil, fmt.Errorf("GitHub 与内网服务均无法完成检查：GitHub: %v；内网: %w", githubErr, intranetErr)
}

func CheckGitHubContext(ctx context.Context, current string) (*UpdateStatus, error) {
	return checkGitHub(ctx, current, latestReleaseURL, allReleasesURL)
}

func checkGitHub(ctx context.Context, current, latestURL, historyURL string) (*UpdateStatus, error) {
	var release ReleaseInfo
	if err := fetchJSONContext(ctx, latestURL, &release); err != nil {
		return nil, err
	}
	if !servicecenter.ValidVersion(release.TagName) {
		return nil, fmt.Errorf("GitHub 版本信息无效")
	}
	downloadURL := selectDownloadURL(release.Assets)
	if downloadURL == "" {
		return nil, fmt.Errorf("GitHub 版本缺少 Windows 安装包")
	}
	hasUpdate := compareVersions(strings.TrimPrefix(current, "v"), strings.TrimPrefix(release.TagName, "v")) < 0
	status := &UpdateStatus{CurrentVer: current, LatestVer: release.TagName, Source: "github", Release: &release, HasUpdate: hasUpdate, DownloadURL: downloadURL}
	if hasUpdate {
		var history []ReleaseInfo
		if err := fetchJSONContext(ctx, historyURL, &history); err == nil {
			status.SkippedVersions, status.Release.Body = cumulativeChangelog(strings.TrimPrefix(current, "v"), &release, history)
		}
	}
	return status, nil
}

// FetchPreferredHistory uses the same source order and total wait budget as
// CheckPreferred. History failure does not affect a successful version check.
func FetchPreferredHistory(parent context.Context, base string) ([]ReleaseInfo, error) {
	ctx, cancel := context.WithTimeout(parent, checkBudget)
	defer cancel()
	githubBudget := checkBudget
	if base != "" {
		githubBudget = githubBudgetWithFallback
	}
	githubCtx, stopGithub := context.WithTimeout(ctx, githubBudget)
	var releases []ReleaseInfo
	githubErr := fetchJSONContext(githubCtx, allReleasesURL, &releases)
	stopGithub()
	if githubErr == nil {
		if len(releases) > 20 {
			releases = releases[:20]
		}
		return releases, nil
	}
	if base == "" {
		return nil, githubErr
	}
	return IntranetHistoryContext(ctx, base)
}

func fetchJSONContext(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := newHTTPClient(0).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("更新服务返回 %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out)
}
