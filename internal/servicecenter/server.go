package servicecenter

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
	protocol "opscopilot/pkg/servicecenter"
)

type Config struct {
	DataDir, PublicURL, AdminToken, GitHubToken, GitHubURL string
	LocalHTTP                                              bool
}
type Server struct {
	cfg        Config
	db         *bolt.DB
	client     *http.Client
	syncing    sync.Mutex
	mux        *http.ServeMux
	syncingNow atomic.Bool
	ctx        context.Context
	stop       context.CancelFunc
}
type Feedback struct {
	ID         string    `json:"id"`
	TokenHash  string    `json:"-"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"createdAt"`
	Status     string    `json:"status"`
	Reply      string    `json:"reply"`
	Attachment string    `json:"attachment,omitempty"`
}

// Stored feedback credentials are never returned by the admin API.
type storedFeedback struct {
	Feedback
	TokenHash string `json:"tokenHash"`
}
type SyncStatus struct {
	LastSuccess  time.Time `json:"lastSuccess"`
	Error        string    `json:"error"`
	StorageBytes int64     `json:"storageBytes"`
	InProgress   bool      `json:"inProgress"`
}
type Aggregate struct {
	Reporting                                            map[string]int `json:"reporting,omitempty"`
	Usage                                                map[string]int `json:"usage,omitempty"`
	Date                                                 string         `json:"date"`
	Active, Connections, Connected, CtrlK, QuickCommands int
	Upgrades                                             map[string]int `json:"upgrades"`
}

func New(cfg Config) (*Server, error) {
	base, err := protocol.ValidateBase(cfg.PublicURL)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(base, "http://") && !cfg.LocalHTTP {
		return nil, fmt.Errorf("HTTP 服务地址需要设置 OPS_SERVICE_LOCAL_HTTP=true")
	}
	if base == "" || len(cfg.AdminToken) < 32 {
		return nil, fmt.Errorf("public URL and admin token (32+ characters) required")
	}
	cfg.PublicURL = base
	if cfg.GitHubURL == "" {
		cfg.GitHubURL = "https://api.github.com/repos/Tudou77826/OpsCopilot/releases/latest"
	}
	if err = os.MkdirAll(filepath.Join(cfg.DataDir, "downloads"), 0700); err != nil {
		return nil, err
	}
	db, err := bolt.Open(filepath.Join(cfg.DataDir, "service.db"), 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, db: db, client: &http.Client{Timeout: 15 * time.Minute}, mux: http.NewServeMux()}
	s.ctx, s.stop = context.WithCancel(context.Background())
	err = db.Update(func(tx *bolt.Tx) error {
		for _, n := range []string{"releases", "announcements", "feedback", "events", "aggregates", "state"} {
			if _, e := tx.CreateBucketIfNotExists([]byte(n)); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	s.routes()
	return s, nil
}
func (s *Server) Close() error {
	s.stop()
	s.syncing.Lock()
	defer s.syncing.Unlock()
	return s.db.Close()
}
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		s.mux.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func decode(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}
func (s *Server) put(bucket, key string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket([]byte(bucket)).Put([]byte(key), b) })
}
func (s *Server) get(bucket, key string, v any) error {
	return s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket)).Get([]byte(key))
		if b == nil {
			return os.ErrNotExist
		}
		return json.Unmarshal(b, v)
	})
}
func (s *Server) list(bucket string, v any) error {
	var rows []json.RawMessage
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucket)).ForEach(func(k, b []byte) error { rows = append(rows, append(json.RawMessage(nil), b...)); return nil })
	})
	if err != nil {
		return err
	}
	if rows == nil {
		rows = []json.RawMessage{}
	}
	b, _ := json.Marshal(rows)
	return json.Unmarshal(b, v)
}
func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.AdminToken)) != 1 {
			respond(w, 401, map[string]string{"error": "authentication required"})
			return
		}
		if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.cfg.PublicURL {
			respond(w, 403, nil)
			return
		}
		next(w, r)
	}
}
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		e := s.db.View(func(tx *bolt.Tx) error { return nil })
		if e != nil {
			respond(w, 503, nil)
			return
		}
		respond(w, 200, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("GET /api/v1/telemetry-policy", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, protocol.Policy{Version: protocol.PolicyVersion, Notice: protocol.Notice})
	})
	s.mux.HandleFunc("GET /api/v1/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		var release protocol.ReleaseInfo
		if s.get("state", "latest", &release) != nil {
			respond(w, 503, map[string]string{"error": "尚无完整同步版本"})
			return
		}
		respond(w, 200, release)
	})
	s.mux.HandleFunc("GET /api/v1/releases", func(w http.ResponseWriter, r *http.Request) {
		var releases []protocol.ReleaseInfo
		if s.list("releases", &releases) != nil {
			respond(w, 500, nil)
			return
		}
		sort.Slice(releases, func(i, j int) bool { return releases[i].PublishedAt.After(releases[j].PublishedAt) })
		rows := make([]portalRelease, 0, len(releases))
		for _, release := range releases {
			rows = append(rows, portalRelease{ReleaseInfo: release, BodyHTML: renderReleaseNotes(release.Body)})
		}
		respond(w, 200, rows)
	})
	s.mux.HandleFunc("GET /downloads/{version}/{filename}", s.download)
	s.mux.HandleFunc("GET /api/v1/announcements", func(w http.ResponseWriter, r *http.Request) {
		var all []protocol.Announcement
		if s.list("announcements", &all) != nil {
			respond(w, 500, nil)
			return
		}
		active := []protocol.Announcement{}
		for _, a := range all {
			if a.Active(time.Now()) {
				active = append(active, a)
			}
		}
		sort.Slice(active, func(i, j int) bool { return active[i].Order < active[j].Order })
		respond(w, 200, active)
	})
	s.mux.HandleFunc("POST /api/v1/events/batch", s.events)
	s.mux.HandleFunc("POST /api/v1/feedback", s.createFeedback)
	s.mux.HandleFunc("POST /api/v1/feedback/lookup", s.lookupFeedback)
	s.mux.HandleFunc("GET /api/admin/status", s.admin(func(w http.ResponseWriter, r *http.Request) {
		var status SyncStatus
		_ = s.get("state", "sync", &status)
		status.InProgress = s.syncingNow.Load()
		_ = filepath.Walk(s.cfg.DataDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				status.StorageBytes += info.Size()
			}
			return nil
		})
		respond(w, 200, status)
	}))
	s.mux.HandleFunc("POST /api/admin/sync", s.admin(func(w http.ResponseWriter, r *http.Request) {
		go s.Sync(context.Background())
		respond(w, 202, map[string]string{"status": "同步已排队"})
	}))
	s.mux.HandleFunc("GET /api/admin/announcements", s.admin(func(w http.ResponseWriter, r *http.Request) {
		var all []protocol.Announcement
		_ = s.list("announcements", &all)
		respond(w, 200, all)
	}))
	s.mux.HandleFunc("POST /api/admin/announcements", s.admin(s.saveAnnouncement))
	s.mux.HandleFunc("DELETE /api/admin/announcements/{id}", s.admin(func(w http.ResponseWriter, r *http.Request) {
		err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket([]byte("announcements")).Delete([]byte(r.PathValue("id"))) })
		if err != nil {
			respond(w, 500, nil)
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
	}))
	s.mux.HandleFunc("GET /api/admin/feedback", s.admin(func(w http.ResponseWriter, r *http.Request) {
		var stored []storedFeedback
		_ = s.list("feedback", &stored)
		all := []Feedback{}
		for _, f := range stored {
			all = append(all, f.Feedback)
		}
		respond(w, 200, all)
	}))
	s.mux.HandleFunc("POST /api/admin/feedback/{id}", s.admin(s.replyFeedback))
	s.mux.HandleFunc("GET /api/admin/feedback/{id}/attachment", s.admin(s.feedbackAttachment))
	s.mux.HandleFunc("GET /api/admin/stats", s.admin(s.stats))
	s.mux.HandleFunc("GET /app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		io.WriteString(w, webJS)
	})
	s.mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		io.WriteString(w, webCSS)
	})
	s.portalRoutes()
	s.adminPages()
	for _, path := range []string{"/help", "/feedback"} {
		p := path
		s.mux.HandleFunc("GET "+p, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != p {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, webHTML)
		})
	}
}
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	var release protocol.ReleaseInfo
	if s.get("releases", r.PathValue("version"), &release) != nil {
		http.NotFound(w, r)
		return
	}
	for _, a := range release.Assets {
		if a.Name == r.PathValue("filename") {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", a.Name))
			http.ServeFile(w, r, filepath.Join(s.cfg.DataDir, "downloads", release.TagName, a.Name))
			return
		}
	}
	http.NotFound(w, r)
}
func (s *Server) saveAnnouncement(w http.ResponseWriter, r *http.Request) {
	var a protocol.Announcement
	if decode(w, r, &a, 16384) != nil || strings.TrimSpace(a.Text) == "" || len(a.Text) > 1000 || !a.EndsAt.After(a.StartsAt) || (a.URL != "" && !protocol.InternalURL(s.cfg.PublicURL, a.URL)) {
		respond(w, 400, map[string]string{"error": "公告内容、有效期或内网链接无效"})
		return
	}
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if len(a.ID) > 100 {
		respond(w, 400, nil)
		return
	}
	if s.put("announcements", a.ID, a) != nil {
		respond(w, 500, nil)
		return
	}
	respond(w, 200, a)
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	var batch struct {
		Records []protocol.Record `json:"records"`
	}
	if decode(w, r, &batch, 1<<20) != nil || len(batch.Records) == 0 || len(batch.Records) > 1000 {
		respond(w, 400, nil)
		return
	}
	for _, v := range batch.Records {
		if v.Validate(time.Now()) != nil {
			respond(w, 400, map[string]string{"error": "记录或授权无效"})
			return
		}
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("events"))
		for _, v := range batch.Records {
			key := []byte(v.ID)
			if prev := b.Get(key); prev != nil {
				var old protocol.Record
				if json.Unmarshal(prev, &old) != nil {
					return fmt.Errorf("corrupt record")
				}
				if old.InstallationID != v.InstallationID || old.Date != v.Date || old.Version != v.Version || old.Kind != v.Kind || old.ReportingMode != v.ReportingMode {
					return fmt.Errorf("record conflict")
				}
				if v.Kind == "daily" {
					v.Active = max(v.Active, old.Active)
					v.Connections = max(v.Connections, old.Connections)
					v.Connected = max(v.Connected, old.Connected)
					v.CtrlK = max(v.CtrlK, old.CtrlK)
					v.QuickCommands = max(v.QuickCommands, old.QuickCommands)
					if v.Usage == nil {
						v.Usage = map[string]int{}
					}
					for k, n := range old.Usage {
						v.Usage[k] = max(v.Usage[k], n)
					}
				} else {
					continue
				}
			}
			data, _ := json.Marshal(v)
			if e := b.Put(key, data); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		respond(w, 409, map[string]string{"error": "记录冲突或写入失败"})
		return
	}
	respond(w, 200, map[string]bool{"ok": true})
}
func hash(token string) string { v := sha256.Sum256([]byte(token)); return hex.EncodeToString(v[:]) }
func (s *Server) createFeedback(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	if r.ParseMultipartForm(6<<20) != nil {
		respond(w, 400, nil)
		return
	}
	defer r.MultipartForm.RemoveAll()
	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" || len(content) > 10000 || r.FormValue("confirmed") != "yes" {
		respond(w, 400, map[string]string{"error": "请确认发送内容"})
		return
	}
	token := uuid.NewString() + uuid.NewString()
	f := storedFeedback{Feedback: Feedback{ID: uuid.NewString(), Content: content, CreatedAt: time.Now(), Status: "待处理"}, TokenHash: hash(token)}
	if file, header, err := r.FormFile("attachment"); err == nil {
		defer file.Close()
		if header.Size > 5<<20 {
			respond(w, 400, nil)
			return
		}
		dir := filepath.Join(s.cfg.DataDir, "feedback")
		if os.MkdirAll(dir, 0700) != nil {
			respond(w, 500, nil)
			return
		}
		out, e := os.OpenFile(filepath.Join(dir, f.ID), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			respond(w, 500, nil)
			return
		}
		_, e = io.Copy(out, file)
		closeErr := out.Close()
		if e != nil || closeErr != nil {
			os.Remove(filepath.Join(dir, f.ID))
			respond(w, 500, nil)
			return
		}
		f.Attachment = "用户附件"
	}
	if s.put("feedback", f.ID, f) != nil {
		os.Remove(filepath.Join(s.cfg.DataDir, "feedback", f.ID))
		respond(w, 500, nil)
		return
	}
	respond(w, 201, map[string]string{"id": f.ID, "token": token})
}
func (s *Server) lookupFeedback(w http.ResponseWriter, r *http.Request) {
	var q struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if decode(w, r, &q, 4096) != nil {
		respond(w, 400, nil)
		return
	}
	var f storedFeedback
	if s.get("feedback", q.ID, &f) != nil || subtle.ConstantTimeCompare([]byte(hash(q.Token)), []byte(f.TokenHash)) != 1 {
		respond(w, 404, nil)
		return
	}
	respond(w, 200, f.Feedback)
}
func (s *Server) replyFeedback(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Status string `json:"status"`
		Reply  string `json:"reply"`
	}
	if decode(w, r, &q, 16384) != nil || len(q.Reply) > 10000 || (q.Status != "待处理" && q.Status != "处理中" && q.Status != "已解决") {
		respond(w, 400, nil)
		return
	}
	var f storedFeedback
	if s.get("feedback", r.PathValue("id"), &f) != nil {
		respond(w, 404, nil)
		return
	}
	f.Status = q.Status
	f.Reply = q.Reply
	if s.put("feedback", f.ID, f) != nil {
		respond(w, 500, nil)
		return
	}
	respond(w, 200, f.Feedback)
}
func (s *Server) feedbackAttachment(w http.ResponseWriter, r *http.Request) {
	var f storedFeedback
	if s.get("feedback", r.PathValue("id"), &f) != nil || f.Attachment == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=feedback-attachment.bin")
	http.ServeFile(w, r, filepath.Join(s.cfg.DataDir, "feedback", f.ID))
}
func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	var events []protocol.Record
	var aggregates []Aggregate
	_ = s.list("events", &events)
	_ = s.list("aggregates", &aggregates)
	versions := map[string]int{}
	seen := map[string]bool{}
	for _, r := range events {
		if r.Kind == "daily" && !seen[r.InstallationID+":"+r.Version] {
			versions[r.Version]++
			seen[r.InstallationID+":"+r.Version] = true
		}
	}
	daily := aggregate(events)
	for _, a := range aggregates {
		daily[a.Date] = a
	}
	respond(w, 200, map[string]any{"coverage": "仅代表同意采集的安装实例；安装量不等于人数。最简授权仅计入活跃和版本分布，功能计数与升级结果仅来自常规授权。版本分布按90天内实例/版本去重。", "versions": versions, "daily": daily})
}
func aggregate(events []protocol.Record) map[string]Aggregate {
	out := map[string]Aggregate{}
	seen := map[string]bool{}
	modes := map[string]protocol.Record{}
	for _, r := range events {
		a := out[r.Date]
		a.Date = r.Date
		if a.Upgrades == nil {
			a.Upgrades = map[string]int{}
		}
		if r.Kind == "daily" {
			key := r.InstallationID + ":" + r.Date
			old, exists := modes[key]
			if !exists || r.ConsentAt.After(old.ConsentAt) || (r.ConsentAt.Equal(old.ConsentAt) && r.Version > old.Version) {
				modes[key] = r
			}
			if !seen[r.InstallationID+":"+r.Date] {
				a.Active += r.Active
				seen[r.InstallationID+":"+r.Date] = true
			}
			a.Connections += r.Connections
			a.Connected += r.Connected
			a.CtrlK += r.CtrlK
			a.QuickCommands += r.QuickCommands
			if a.Usage == nil {
				a.Usage = map[string]int{}
			}
			for k, n := range r.Usage {
				a.Usage[k] += n
			}
		} else {
			a.Upgrades[r.Phase+":"+r.Result]++
		}
		out[r.Date] = a
	}
	for _, r := range modes {
		a := out[r.Date]
		if a.Reporting == nil {
			a.Reporting = map[string]int{"standard": 0, "minimal": 0}
		}
		if r.ReportingMode == "standard" || r.ReportingMode == "minimal" {
			a.Reporting[r.ReportingMode]++
		}
		out[r.Date] = a
	}
	return out
}

// Maintenance converts expiring instance records into anonymous daily totals in one transaction.
func (s *Server) Maintenance(now time.Time) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("events"))
		var expired []protocol.Record
		var keys [][]byte
		e := b.ForEach(func(k, v []byte) error {
			var r protocol.Record
			if e := json.Unmarshal(v, &r); e != nil {
				return e
			}
			if r.Date < now.UTC().AddDate(0, 0, -90).Format("2006-01-02") {
				expired = append(expired, r)
				keys = append(keys, append([]byte(nil), k...))
			}
			return nil
		})
		if e != nil {
			return e
		}
		for day, a := range aggregate(expired) {
			data, _ := json.Marshal(a)
			if e = tx.Bucket([]byte("aggregates")).Put([]byte(day), data); e != nil {
				return e
			}
		}
		for _, k := range keys {
			if e = b.Delete(k); e != nil {
				return e
			}
		}
		b = tx.Bucket([]byte("aggregates"))
		c := b.Cursor()
		cut := now.UTC().AddDate(-1, 0, 0).Format("2006-01-02")
		for k, _ := c.First(); k != nil && string(k) < cut; k, _ = c.Next() {
			if e = c.Delete(); e != nil {
				return e
			}
		}
		return nil
	})
}
