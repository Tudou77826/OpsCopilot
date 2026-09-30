package servicecenter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"opscopilot/pkg/filetxn"
)

type State struct {
	BaseURL       string    `json:"baseUrl"`
	Choice        string    `json:"choice"`
	PolicyVersion string    `json:"policyVersion"`
	ConsentAt     time.Time `json:"consentAt"`
	// Explicit empty values allow three-way persistence to remove previous data.
	ConsentRecipient string         `json:"consentRecipient"`
	InstallationID   string         `json:"installationId"`
	Records          []Record       `json:"records"`
	Pending          *Record        `json:"pending"`
	Announcements    []Announcement `json:"announcements"`
}
type Settings struct {
	BaseURL      string `json:"baseUrl"`
	Choice       string `json:"choice"`
	NeedsConsent bool   `json:"needsConsent"`
	Policy       Policy `json:"policy"`
	Ready        bool   `json:"ready"`
}
type Client struct {
	mu            sync.Mutex
	state         State
	path, version string
	http          *http.Client
	readyUntil    time.Time
	cancel        context.CancelFunc
	epoch         uint64
	diskBase      []byte
}

func NewClient(path, version string) *Client {
	c := &Client{path: path, version: version, http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || !InternalURL(via[0].URL.Scheme+"://"+via[0].URL.Host, req.URL.String()) {
			return fmt.Errorf("服务请求禁止跨站跳转")
		}
		return nil
	}}}
	if data, e := os.ReadFile(path); e == nil {
		if json.Unmarshal(data, &c.state) == nil {
			c.diskBase = data
		} else {
			c.state = State{}
		}
	}
	base, e := ValidateBase(c.state.BaseURL)
	if e != nil {
		c.state = State{}
	} else {
		c.state.BaseURL = base
	}
	if !reportingEnabled(c.state.Choice) && c.state.Choice != "disabled" {
		c.state.Choice = ""
	}
	if reportingEnabled(c.state.Choice) && (c.state.PolicyVersion != consentVersion(c.state.Choice) || c.state.InstallationID == "" || c.state.ConsentAt.IsZero() || c.state.ConsentRecipient != c.state.BaseURL) {
		c.state.Choice = ""
	}
	if !reportingEnabled(c.state.Choice) {
		c.clearLocked()
	}
	c.pruneLocked()
	return c
}
func (c *Client) settingsLocked() Settings {
	return Settings{BaseURL: c.state.BaseURL, Choice: c.state.Choice, NeedsConsent: c.state.Choice == "", Policy: Policy{Version: PolicyVersion, Notice: Notice}, Ready: c.allowedLocked()}
}
func (c *Client) Settings() Settings {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reloadLocked()
	return c.settingsLocked()
}
func (c *Client) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return err
	}
	data, e := json.Marshal(c.state)
	if e != nil {
		return e
	}
	merged, e := filetxn.Merge(c.path, c.diskBase, data)
	if e == nil {
		c.diskBase = merged
	}
	return e
}

// Observe consent changes from other desktop windows before any collection or send.
func (c *Client) reloadLocked() {
	data, e := os.ReadFile(c.path)
	if e != nil {
		if len(c.diskBase) > 0 {
			c.clearLocked()
			c.state.Choice = ""
			c.diskBase = nil
		}
		return
	}
	if bytes.Equal(data, c.diskBase) {
		return
	}
	var state State
	if json.Unmarshal(data, &state) != nil {
		c.clearLocked()
		c.state.Choice = ""
		return
	}
	base, err := ValidateBase(state.BaseURL)
	if err != nil {
		state = State{}
	} else {
		state.BaseURL = base
	}
	if reportingEnabled(state.Choice) && (state.ConsentRecipient != state.BaseURL || state.PolicyVersion != consentVersion(state.Choice) || state.ConsentAt.IsZero() || state.InstallationID == "") {
		state.Choice = ""
		state.Records = nil
		state.Pending = nil
		state.InstallationID = ""
	}
	changed := state.Choice != c.state.Choice || state.InstallationID != c.state.InstallationID || state.BaseURL != c.state.BaseURL || state.PolicyVersion != c.state.PolicyVersion
	if changed {
		c.clearLocked()
	}
	c.state = state
	c.diskBase = data
	if !reportingEnabled(c.state.Choice) {
		c.state.Records = nil
		c.state.Pending = nil
		c.state.InstallationID = ""
	}
	c.pruneLocked()
}
func (c *Client) clearLocked() {
	c.epoch++
	c.readyUntil = time.Time{}
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.state.InstallationID = ""
	c.state.Records = nil
	c.state.Pending = nil
	c.state.ConsentAt = time.Time{}
	c.state.PolicyVersion = ""
	c.state.ConsentRecipient = ""
}
func (c *Client) Configure(base string) (Settings, error) {
	normalized, e := ValidateBase(base)
	if e != nil {
		return Settings{}, e
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	release, lockErr := filetxn.Lock(c.path)
	if lockErr != nil {
		return Settings{}, lockErr
	}
	defer release()
	c.reloadLocked()
	if normalized != c.state.BaseURL {
		c.clearLocked()
		c.state.BaseURL = normalized
		c.state.Announcements = nil
		if c.state.Choice != "disabled" {
			c.state.Choice = ""
		}
	}
	e = c.writeLocked()
	if e != nil {
		c.clearLocked()
		c.state.Choice = ""
	}
	return c.settingsLocked(), e
}
func (c *Client) Choose(choice string) (Settings, error) {
	if !reportingEnabled(choice) && choice != "disabled" {
		return Settings{}, fmt.Errorf("无效授权选择")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	release, lockErr := filetxn.Lock(c.path)
	if lockErr != nil {
		return Settings{}, lockErr
	}
	defer release()
	c.reloadLocked()
	c.clearLocked()
	c.state.Choice = choice
	if reportingEnabled(choice) {
		c.state.InstallationID = uuid.NewString()
		c.state.ConsentAt = time.Now().UTC()
		c.state.PolicyVersion = consentVersion(choice)
		c.state.ConsentRecipient = c.state.BaseURL
	}
	e := c.writeLocked()
	if e != nil {
		c.clearLocked()
		c.state.Choice = ""

	}
	return c.settingsLocked(), e
}
func (c *Client) allowedLocked() bool {
	c.reloadLocked()
	return reportingEnabled(c.state.Choice) && c.state.BaseURL != "" && c.state.ConsentRecipient == c.state.BaseURL && c.state.PolicyVersion == consentVersion(c.state.Choice) && time.Now().Before(c.readyUntil)
}
func (c *Client) pruneLocked() {
	cut := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	if c.state.Pending != nil && c.state.Pending.Date < cut {
		c.state.Pending = nil
	}
	keep := []Record{}
	for _, r := range c.state.Records {
		if r.Date >= cut {
			keep = append(keep, r)
		}
	}
	if len(keep) > 1000 {
		keep = keep[len(keep)-1000:]
	}
	c.state.Records = keep
}
func (c *Client) baseRecordLocked() Record {
	r := Record{ReportingMode: c.state.Choice, InstallationID: c.state.InstallationID, Date: time.Now().Format("2006-01-02"), Version: c.version, OS: runtime.GOOS, Arch: runtime.GOARCH, PolicyVersion: c.state.PolicyVersion, ConsentAt: c.state.ConsentAt}
	if c.state.Choice == "minimal" {
		r.OS = ""
		r.Arch = ""
	}
	return r
}
func (c *Client) Count(kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.countLocked(kind)
}
func (c *Client) countLocked(kind string, expected ...string) string {
	if !c.allowedLocked() {
		return ""
	}
	release, err := filetxn.Lock(c.path)
	if err != nil {
		return ""
	}
	defer release()
	c.reloadLocked()
	if len(expected) > 0 && c.state.InstallationID != expected[0] {
		return ""
	}
	if !c.allowedLocked() {
		return ""
	}
	if c.state.Choice == "minimal" && kind != "active" {
		return ""
	}
	if kind != "active" && kind != "connection" && kind != "connected" && kind != "ctrl_k" && kind != "quick_command" && !ValidUsage(kind) {
		return ""
	}
	r := c.baseRecordLocked()
	r.Kind = "daily"
	r.ID = r.InstallationID + ":" + r.Date + ":" + r.Version
	idx := -1
	for i, v := range c.state.Records {
		if v.ID == r.ID {
			r = v
			idx = i
			break
		}
	}
	switch kind {
	case "active":
		r.Active = 1
	case "connection":
		r.Connections++
	case "connected":
		if r.Connected < r.Connections {
			r.Connected++
		}
	case "ctrl_k":
		r.CtrlK++
	case "quick_command":
		r.QuickCommands++
	default:
		if r.Usage == nil {
			r.Usage = map[string]int{}
		}
		if r.Usage[kind] < 1000000 {
			r.Usage[kind]++
		}
	}
	if idx < 0 {
		c.state.Records = append(c.state.Records, r)
	} else {
		c.state.Records[idx] = r
	}
	c.pruneLocked()
	if c.writeLocked() != nil {
		c.readyUntil = time.Time{}
		return ""
	}
	return r.ID
}

// Each deliberate attempt gets one completion callback. Reconsent never adopts an old attempt.
func (c *Client) BeginConnection() func(bool) {
	c.mu.Lock()
	id := c.countLocked("connection")
	epoch := c.epoch
	installation := c.state.InstallationID
	c.mu.Unlock()
	var once sync.Once
	return func(success bool) {
		once.Do(func() {
			if !success || id == "" {
				return
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if !c.allowedLocked() || epoch != c.epoch {
				return
			}
			if id == installation+":"+time.Now().Format("2006-01-02")+":"+c.version {
				c.countLocked("connected", installation)
			}
		})
	}
}

func (c *Client) Upgrade(id, target, phase, result, errorCode string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.allowedLocked() || c.state.Choice != "standard" {
		return
	}
	r := c.baseRecordLocked()
	r.Kind = "upgrade"
	r.UpgradeID = id
	r.ID = id + ":" + phase
	r.TargetVersion = target
	r.Phase = phase
	r.Result = result
	r.ErrorCode = errorCode
	if r.Validate(time.Now()) != nil {
		return
	}
	found := false
	for i, v := range c.state.Records {
		if v.ID == r.ID {
			c.state.Records[i] = r
			found = true
			break
		}
	}
	if !found {
		c.state.Records = append(c.state.Records, r)
	}
	c.pruneLocked()
	if c.saveLocked() != nil {
		c.readyUntil = time.Time{}
	}
}
func (c *Client) PendingUpgrade(id, target string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.allowedLocked() || c.state.Choice != "standard" {
		return nil
	}
	r := c.baseRecordLocked()
	r.Kind = "upgrade"
	r.TargetVersion = target
	r.UpgradeID = id
	c.state.Pending = &r
	return c.saveLocked()
}
func (c *Client) Run(ctx context.Context) {
	c.Refresh(ctx)
	tick := time.NewTicker(time.Minute)
	watch := time.NewTicker(time.Second)
	defer watch.Stop()
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			c.mu.Lock()
			c.readyUntil = time.Time{}
			if c.cancel != nil {
				c.cancel()
			}
			c.mu.Unlock()
			return
		case <-tick.C:
			c.Refresh(ctx)
		case <-watch.C:
			c.mu.Lock()
			c.reloadLocked()
			c.mu.Unlock()
		}
	}
}
func (c *Client) Refresh(parent context.Context) {
	c.mu.Lock()
	c.reloadLocked()
	if !reportingEnabled(c.state.Choice) || c.state.BaseURL == "" {
		c.mu.Unlock()
		return
	}
	if c.cancel != nil {
		c.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	base := c.state.BaseURL
	epoch := c.epoch
	c.readyUntil = time.Time{}
	c.mu.Unlock()
	defer func() {
		cancel()
		c.mu.Lock()
		if c.epoch == epoch {
			c.cancel = nil
		}
		c.mu.Unlock()
	}()
	var policy Policy
	if c.get(ctx, base+"/api/v1/telemetry-policy", &policy) != nil {
		return
	}
	c.mu.Lock()
	if c.epoch != epoch {
		c.mu.Unlock()
		return
	}
	if policy.Version != PolicyVersion {
		// Invalidation changes consent and must share the same disk transaction as
		// Choose. Reload under the lock so a newer choice cannot be overwritten.
		release, lockErr := filetxn.Lock(c.path)
		if lockErr == nil {
			c.reloadLocked()
			if c.epoch != epoch {
				release()
				c.mu.Unlock()
				return
			}
		}
		c.clearLocked()
		c.state.Choice = ""
		if lockErr == nil {
			_ = c.writeLocked()
			release()
		}
		c.mu.Unlock()
		return
	}
	c.readyUntil = time.Now().Add(70 * time.Second)
	pending := c.state.Pending
	c.mu.Unlock()
	c.Count("active")
	if pending != nil && pending.TargetVersion == c.version {
		c.Upgrade(pending.UpgradeID, pending.TargetVersion, "startup", "success", "none")
		c.mu.Lock()
		if c.epoch == epoch {
			c.state.Pending = nil
			_ = c.saveLocked()
		}
		c.mu.Unlock()
	}
	c.mu.Lock()
	if c.epoch != epoch || !c.allowedLocked() {
		c.mu.Unlock()
		return
	}
	records := cloneRecords(c.state.Records)
	c.mu.Unlock()
	if len(records) == 0 {
		return
	}
	data, _ := json.Marshal(struct {
		Records []Record `json:"records"`
	}{records})
	req, e := http.NewRequestWithContext(ctx, "POST", base+"/api/v1/events/batch", bytes.NewReader(data))
	if e != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.http.Do(req)
	if e != nil {
		c.mu.Lock()
		if c.epoch == epoch {
			c.readyUntil = time.Time{}
		}
		c.mu.Unlock()
		return
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		c.mu.Lock()
		if c.epoch == epoch {
			c.readyUntil = time.Time{}
		}
		c.mu.Unlock()
		return
	}
	// Retain current day's cumulative counters for subsequent idempotent snapshots.
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epoch != epoch {
		return
	}
	keep := []Record{}
	today := time.Now().Format("2006-01-02")
	sent := map[string]Record{}
	for _, r := range records {
		sent[r.ID] = r
	}
	for _, r := range c.state.Records {
		previous, ok := sent[r.ID]
		if !ok || !reflect.DeepEqual(previous, r) || (r.Kind == "daily" && r.Date == today) {
			keep = append(keep, r)
		}
	}
	c.state.Records = keep
	_ = c.saveLocked()
}

// TestConnection checks the entered address without saving settings or collecting data.
func (c *Client) TestConnection(ctx context.Context, raw string) error {
	base, err := ValidateBase(raw)
	if err != nil || base == "" {
		return fmt.Errorf("请填写管理员提供的 HTTP 或 HTTPS 服务地址")
	}
	var policy Policy
	if err := c.get(ctx, base+"/api/v1/telemetry-policy", &policy); err != nil {
		return fmt.Errorf("无法连接内网服务，请检查地址、网络和证书，或联系管理员")
	}
	if policy.Version != PolicyVersion {
		return fmt.Errorf("服务版本与客户端不兼容，请联系管理员")
	}
	return nil
}

func (c *Client) get(ctx context.Context, url string, out any) error {
	req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
	if e != nil {
		return e
	}
	resp, e := c.http.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("service status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out)
}
func (c *Client) Announcements(ctx context.Context) []Announcement {
	c.mu.Lock()
	base := c.state.BaseURL
	c.mu.Unlock()
	if base == "" {
		return []Announcement{}
	}
	var all []Announcement
	err := c.get(ctx, base+"/api/v1/announcements", &all)
	c.mu.Lock()
	defer c.mu.Unlock()
	if base != c.state.BaseURL {
		return []Announcement{}
	}
	if err == nil {
		safe := []Announcement{}
		for _, a := range all {
			if len(a.Text) <= 1000 && (a.URL == "" || InternalURL(base, a.URL)) {
				safe = append(safe, a)
			}
		}
		c.state.Announcements = safe
		_ = c.saveLocked()
	}
	active := []Announcement{}
	for _, a := range c.state.Announcements {
		if a.Active(time.Now()) {
			active = append(active, a)
		}
	}
	return active
}

func (c *Client) writeLocked() error {
	data, err := json.Marshal(c.state)
	if err != nil {
		return err
	}
	if err = filetxn.Write(c.path, data); err == nil {
		c.diskBase = data
	}
	return err
}
func cloneRecords(records []Record) []Record {
	out := append([]Record(nil), records...)
	for i := range out {
		if records[i].Usage != nil {
			out[i].Usage = map[string]int{}
			for k, v := range records[i].Usage {
				out[i].Usage[k] = v
			}
		}
	}
	return out
}

// BeginUsage binds completion to the original consent and counts each outcome once.
func (c *Client) BeginUsage(prefix string) func(string) {
	c.mu.Lock()
	id := c.countLocked(prefix + "_started")
	epoch := c.epoch
	installation := c.state.InstallationID
	c.mu.Unlock()
	var once sync.Once
	return func(outcome string) {
		once.Do(func() {
			if id == "" || !oneOf(outcome, "success", "failure", "cancelled") {
				return
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			c.reloadLocked()
			if c.epoch == epoch {
				c.countLocked(prefix+"_"+outcome, installation)
			}
		})
	}
}

func (c *Client) Identity() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reloadLocked()
	return c.state.InstallationID
}
func (c *Client) CountForInstallation(kind, installation string) {
	if installation == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.countLocked(kind, installation)
}
