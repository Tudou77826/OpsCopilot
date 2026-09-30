package servicecenter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	protocol "opscopilot/pkg/servicecenter"
)

func safeName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, `/\:<>"|?*`)
}
func (s *Server) Sync(ctx context.Context) (err error) {
	if !s.syncing.TryLock() {
		return nil
	}
	defer s.syncing.Unlock()
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	s.syncingNow.Store(true)
	defer s.syncingNow.Store(false)
	syncCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	ctx = syncCtx
	defer func() {
		var status SyncStatus
		_ = s.get("state", "sync", &status)
		if err != nil {
			status.Error = "GitHub 同步失败，请检查网络、磁盘及附件完整性"
		} else {
			status.LastSuccess = time.Now()
			status.Error = ""
		}
		_ = s.put("state", "sync", status)
	}()
	req, err := http.NewRequestWithContext(ctx, "GET", s.cfg.GitHubURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if s.cfg.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.GitHubToken)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub status %d", resp.StatusCode)
	}
	var upstream struct {
		protocol.ReleaseInfo
		Draft      bool `json:"draft"`
		Prerelease bool `json:"prerelease"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&upstream); err != nil {
		return err
	}
	release := upstream.ReleaseInfo
	release.Source = "github"
	if upstream.Draft || upstream.Prerelease || !safeName(release.TagName) || !protocol.ValidVersion(release.TagName) || len(release.Assets) == 0 {
		return fmt.Errorf("invalid formal release")
	}
	var cached protocol.ReleaseInfo
	if s.get("releases", release.TagName, &cached) == nil {
		// An administrator may publish the same formal version with fewer optional
		// assets. Keep that immutable upload instead of treating it as a mismatch.
		if cached.Source != "upload" && !sameRelease(cached, release) {
			return fmt.Errorf("published version changed")
		}
		if err := s.verifyCached(cached); err != nil {
			return err
		}
		return s.promoteLatest(cached)
	}
	stage, err := os.MkdirTemp(filepath.Join(s.cfg.DataDir, "downloads"), ".sync-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	seen := map[string]bool{}
	for i, a := range release.Assets {
		if !safeName(a.Name) || seen[strings.ToLower(a.Name)] || a.Size <= 0 || a.Size > 4<<30 {
			return fmt.Errorf("invalid asset")
		}
		seen[strings.ToLower(a.Name)] = true
		u, e := url.Parse(a.BrowserDownloadURL)
		if e != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil {
			return fmt.Errorf("invalid GitHub asset URL")
		}
		req, e := http.NewRequestWithContext(ctx, "GET", a.BrowserDownloadURL, nil)
		if e != nil {
			return e
		}
		res, e := s.client.Do(req)
		if e != nil {
			return e
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			return fmt.Errorf("asset status %d", res.StatusCode)
		}
		f, e := os.OpenFile(filepath.Join(stage, a.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			res.Body.Close()
			return e
		}
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(f, h), io.LimitReader(res.Body, a.Size+1))
		res.Body.Close()
		syncErr := f.Sync()
		closeErr := f.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		if syncErr != nil {
			return syncErr
		}
		digest := hex.EncodeToString(h.Sum(nil))
		if n != a.Size || (a.Digest != "" && a.Digest != "sha256:"+digest) {
			return fmt.Errorf("asset checksum/size mismatch")
		}
		release.Assets[i].SHA256 = digest
		release.Assets[i].BrowserDownloadURL = s.cfg.PublicURL + "/downloads/" + url.PathEscape(release.TagName) + "/" + url.PathEscape(a.Name)
	}
	release.HTMLURL = s.cfg.PublicURL + "/"
	return s.publishRelease(stage, release, false)
}
func (s *Server) verifyCached(r protocol.ReleaseInfo) error {
	for _, a := range r.Assets {
		f, e := os.Open(filepath.Join(s.cfg.DataDir, "downloads", r.TagName, a.Name))
		if e != nil {
			return e
		}
		h := sha256.New()
		n, e := io.Copy(h, f)
		f.Close()
		if e != nil {
			return e
		}
		if n != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
			return fmt.Errorf("cached artifact corrupted")
		}
	}
	return nil
}
func sameRelease(a, b protocol.ReleaseInfo) bool {
	if a.TagName != b.TagName || len(a.Assets) != len(b.Assets) {
		return false
	}
	for i, v := range a.Assets {
		if v.Name != b.Assets[i].Name || v.Size != b.Assets[i].Size || (b.Assets[i].Digest != "" && b.Assets[i].Digest != "sha256:"+v.SHA256) {
			return false
		}
	}
	return true
}
