package servicecenter

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
	protocol "opscopilot/pkg/servicecenter"
)

const maxReleaseUpload = 1 << 30 // 1 GiB for the complete multipart request.
const maxReleaseAsset = 512 << 20

var manualVersion = regexp.MustCompile(`^v(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)
var errReleaseConflict = errors.New("版本已存在或不高于当前版本，请使用新的版本号")

var uploadAssets = map[string]string{
	"windows":     "opscopilot-windows.zip",
	"exe":         "opscopilot.exe",
	"linux_amd64": "opscopilot-service-center-linux-amd64",
	"linux_arm64": "opscopilot-service-center-linux-arm64",
}

// compareReleaseVersion compares the three numeric components used for formal releases.
func compareReleaseVersion(a, b string) (int, error) {
	parse := func(v string) ([3]uint64, error) {
		var out [3]uint64
		if !manualVersion.MatchString(v) {
			return out, fmt.Errorf("invalid version %q", v)
		}
		for i, part := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
			n, err := strconv.ParseUint(part, 10, 64)
			if err != nil {
				return out, err
			}
			out[i] = n
		}
		return out, nil
	}
	x, err := parse(a)
	if err != nil {
		return 0, err
	}
	y, err := parse(b)
	if err != nil {
		return 0, err
	}
	for i := range x {
		if x[i] > y[i] {
			return 1, nil
		}
		if x[i] < y[i] {
			return -1, nil
		}
	}
	return 0, nil
}

func (s *Server) uploadRelease(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
		respond(w, 400, map[string]string{"error": "请选择安装包并填写版本信息"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxReleaseUpload)
	reader, err := r.MultipartReader()
	if err != nil {
		respond(w, 400, map[string]string{"error": "上传格式无效"})
		return
	}
	stage, err := os.MkdirTemp(filepath.Join(s.cfg.DataDir, "downloads"), ".upload-")
	if err != nil {
		respond(w, 500, map[string]string{"error": "无法准备上传目录"})
		return
	}
	defer os.RemoveAll(stage)
	var version, notes string
	seen := map[string]bool{}
	assets := []protocol.Asset{}
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			respond(w, 400, map[string]string{"error": "上传中断或超过 1 GB 限制"})
			return
		}
		name := part.FormName()
		if seen[name] {
			part.Close()
			respond(w, 400, map[string]string{"error": "上传字段重复"})
			return
		}
		seen[name] = true
		switch name {
		case "version", "notes":
			if part.FileName() != "" {
				part.Close()
				respond(w, 400, map[string]string{"error": "版本信息格式无效"})
				return
			}
			b, e := io.ReadAll(io.LimitReader(part, 65537))
			if e != nil || len(b) > 65536 {
				part.Close()
				respond(w, 400, map[string]string{"error": "版本说明超过 64 KB"})
				return
			}
			if name == "version" {
				version = strings.TrimSpace(string(b))
			} else {
				notes = strings.TrimSpace(string(b))
			}
		default:
			filename, ok := uploadAssets[name]
			if !ok || part.FileName() == "" {
				part.Close()
				respond(w, 400, map[string]string{"error": "安装包类型无效"})
				return
			}
			f, e := os.OpenFile(filepath.Join(stage, filename), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				part.Close()
				respond(w, 500, map[string]string{"error": "无法保存安装包"})
				return
			}
			h := sha256.New()
			n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(part, maxReleaseAsset+1))
			part.Close()
			syncErr := f.Sync()
			closeErr := f.Close()
			if copyErr != nil || syncErr != nil || closeErr != nil || n == 0 || n > maxReleaseAsset {
				respond(w, 400, map[string]string{"error": "安装包上传失败、为空或超过 512 MB"})
				return
			}
			assets = append(assets, protocol.Asset{Name: filename, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))})
		}
	}
	if !manualVersion.MatchString(version) || len(version) > 50 || notes == "" || !seen["windows"] {
		respond(w, 400, map[string]string{"error": "请填写 v主.次.修订 版本号、更新说明，并上传 Windows ZIP 安装包"})
		return
	}
	if err := validateReleaseFiles(stage, seen); err != nil {
		respond(w, 400, map[string]string{"error": err.Error()})
		return
	}
	for i := range assets {
		assets[i].BrowserDownloadURL = s.cfg.PublicURL + "/downloads/" + url.PathEscape(version) + "/" + url.PathEscape(assets[i].Name)
	}
	release := protocol.ReleaseInfo{TagName: version, Source: "upload", Name: version, Body: notes, HTMLURL: s.cfg.PublicURL + "/", PublishedAt: time.Now().UTC(), Assets: assets}
	if err := s.publishRelease(stage, release, true); err != nil {
		if errors.Is(err, errReleaseConflict) {
			respond(w, 409, map[string]string{"error": err.Error()})
		} else {
			respond(w, 500, map[string]string{"error": "版本发布失败，请检查存储空间"})
		}
		return
	}
	respond(w, 201, release)
}

func validateReleaseFiles(stage string, seen map[string]bool) error {
	path := filepath.Join(stage, uploadAssets["windows"])
	z, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("Windows 安装包不是有效的 ZIP 文件")
	}
	defer z.Close()
	foundExe := false
	var unpacked uint64
	for _, f := range z.File {
		name := f.Name
		if name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") || filepath.IsAbs(name) || strings.HasPrefix(filepath.Clean(name), "..") || f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Windows 安装包包含不安全的路径")
		}
		unpacked += f.UncompressedSize64
		if unpacked > 4<<30 {
			return fmt.Errorf("Windows 安装包解压体积过大")
		}
		if f.FileInfo().IsDir() {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return fmt.Errorf("Windows 安装包内文件无法读取")
		}
		if name == "opscopilot.exe" {
			var magic [2]byte
			if _, err := io.ReadFull(r, magic[:]); err != nil || string(magic[:]) != "MZ" {
				r.Close()
				return fmt.Errorf("Windows 安装包内的 opscopilot.exe 无效")
			}
			foundExe = true
		}
		_, err = io.Copy(io.Discard, r)
		closeErr := r.Close()
		if err != nil || closeErr != nil {
			return fmt.Errorf("Windows 安装包内文件损坏")
		}
	}
	if !foundExe {
		return fmt.Errorf("Windows 安装包缺少根目录下的 opscopilot.exe")
	}
	for _, key := range []string{"exe", "linux_amd64", "linux_arm64"} {
		if !seen[key] {
			continue
		}
		f, err := os.Open(filepath.Join(stage, uploadAssets[key]))
		if err != nil {
			return err
		}
		var header [20]byte
		_, err = io.ReadFull(f, header[:])
		f.Close()
		machine := uint16(header[18]) | uint16(header[19])<<8
		if err != nil || (key == "exe" && string(header[:2]) != "MZ") || (key == "linux_amd64" && (string(header[:4]) != "\x7fELF" || header[4] != 2 || header[5] != 1 || machine != 62)) || (key == "linux_arm64" && (string(header[:4]) != "\x7fELF" || header[4] != 2 || header[5] != 1 || machine != 183)) {
			return fmt.Errorf("%s 文件格式无效", uploadAssets[key])
		}
	}
	return nil
}

// publishRelease serializes publication only, so a stalled GitHub download cannot block an upload.
func (s *Server) publishRelease(stage string, release protocol.ReleaseInfo, requireNewer bool) error {
	s.publishing.Lock()
	defer s.publishing.Unlock()
	var latest protocol.ReleaseInfo
	if err := s.get("state", "latest", &latest); err != nil && !os.IsNotExist(err) {
		return err
	}
	if latest.TagName != "" {
		cmp, err := compareReleaseVersion(release.TagName, latest.TagName)
		if requireNewer && (err != nil || cmp <= 0) {
			return errReleaseConflict
		}
	}
	var existing protocol.ReleaseInfo
	if err := s.get("releases", release.TagName, &existing); err == nil {
		return errReleaseConflict
	} else if !os.IsNotExist(err) {
		return err
	}
	final := filepath.Join(s.cfg.DataDir, "downloads", release.TagName)
	if _, err := os.Stat(final); err == nil {
		// Only an unpublished directory for this validated tag may be an orphan.
		if err := os.RemoveAll(final); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stage, final); err != nil {
		return err
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(release)
		if err != nil {
			return err
		}
		if err = tx.Bucket([]byte("releases")).Put([]byte(release.TagName), data); err != nil {
			return err
		}
		cmp, cmpErr := compareReleaseVersion(release.TagName, latest.TagName)
		if latest.TagName == "" || (cmpErr == nil && cmp > 0) {
			return tx.Bucket([]byte("state")).Put([]byte("latest"), data)
		}
		return nil
	})
	if err != nil {
		_ = os.RemoveAll(final)
	}
	return err
}

func (s *Server) promoteLatest(release protocol.ReleaseInfo) error {
	s.publishing.Lock()
	defer s.publishing.Unlock()
	var latest protocol.ReleaseInfo
	if err := s.get("state", "latest", &latest); err == nil {
		cmp, err := compareReleaseVersion(release.TagName, latest.TagName)
		if err != nil || cmp <= 0 {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return s.put("state", "latest", release)
}
