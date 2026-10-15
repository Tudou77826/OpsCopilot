package achievements

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	social "opscopilot/pkg/companionsocial"
	"opscopilot/pkg/filetxn"
	"opscopilot/pkg/secretstore"
	"opscopilot/pkg/servicecenter"
)

type Client struct {
	store   *Store
	secrets secretstore.SecretStore
	http    *http.Client
	syncMu  sync.Mutex
}

func NewClient(store *Store, secrets secretstore.SecretStore) *Client {
	return &Client{store: store, secrets: secrets, http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}}
}
func credentialKey(binding Binding, sourceID string) string {
	return ObjectID(binding.BaseURL + "\x00" + string(binding.Member.UserID) + "\x00" + sourceID)
}
func (c *Client) request(ctx context.Context, base, token, method, path string, input, out any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("排名服务暂不可用")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return fmt.Errorf("读取排名响应失败")
	}
	if len(data) > 1<<20 {
		return fmt.Errorf("排名响应过大")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &failure)
		if failure.Error == "" {
			failure.Error = "排名请求未成功"
		}
		return fmt.Errorf("%s（%d）", failure.Error, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err = json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("排名响应格式无效")
	}
	return nil
}
func (c *Client) Connect(ctx context.Context, base, token string, claimLocal bool) (Status, error) {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	if !c.store.Enabled() {
		return Status{}, ErrDisabled
	}
	base, err := servicecenter.ValidateBase(base)
	if err != nil || base == "" {
		return Status{}, fmt.Errorf("排名服务地址无效")
	}
	var response struct {
		Member social.Member `json:"member"`
	}
	if err = c.request(ctx, base, token, http.MethodGet, "/api/v1/social/me", nil, &response); err != nil {
		return Status{}, err
	}
	binding := Binding{BaseURL: base, Member: response.Member}
	// Persist source identity before assigning a Keyring entry; a missing JSON
	// file otherwise produces a new source identity on each read.
	if err = c.store.mutate(func(st *State) error { return nil }); err != nil {
		return Status{}, err
	}
	st, err := c.store.Read()
	if err != nil {
		return Status{}, err
	}
	if err = c.store.Bind(binding, claimLocal); err != nil {
		return Status{}, err
	}
	if err = c.secrets.Set("OpsCopilot-Social", credentialKey(binding, st.SourceID), token); err != nil {
		return Status{}, fmt.Errorf("保存成员凭据失败，请重新连接: %w", err)
	}
	return c.store.Status()
}
func (c *Client) bound() (*State, string, error) {
	st, err := c.store.Read()
	if err != nil {
		return nil, "", err
	}
	if st.Binding == nil {
		return nil, "", fmt.Errorf("请先连接成员账号")
	}
	token, err := c.secrets.Get("OpsCopilot-Social", credentialKey(*st.Binding, st.SourceID))
	if err != nil {
		return nil, "", fmt.Errorf("成员凭据不可用，请重新连接")
	}
	return st, token, nil
}
func (c *Client) Participation(ctx context.Context, enabled bool) error {
	// Disable local upload first. Even if offline, a later retry must not send
	// new records; report failure until server visibility is actually withdrawn.
	if err := c.store.SetParticipation(enabled); err != nil {
		return err
	}
	_, err := c.Sync(ctx)
	return err
}
func (c *Client) Sync(ctx context.Context) (Status, error) {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	if !c.store.Enabled() {
		return Status{}, ErrDisabled
	}
	// Desktop and sidecar serialize requests for their shared source as well as
	// JSON mutations. An older in-flight opt-in cannot overtake a later opt-out.
	release, err := filetxn.Lock(c.store.path + ".sync")
	if err != nil {
		return Status{}, err
	}
	defer release()
	st, token, err := c.bound()
	if err != nil {
		return Status{}, err
	}
	if st.ParticipationPending != nil {
		enabled := *st.ParticipationPending
		var member social.Member
		if err = c.request(ctx, st.Binding.BaseURL, token, http.MethodPut, "/api/v1/social/participation", map[string]bool{"enabled": enabled}, &member); err != nil {
			return Status{}, err
		}
		if member.Participating != enabled {
			return Status{}, fmt.Errorf("参与确认不匹配")
		}
		if err = c.store.AckParticipation(enabled); err != nil {
			return Status{}, err
		}
		st, err = c.store.Read()
		if err != nil {
			return Status{}, err
		}
	}
	if !st.Binding.Member.Participating {
		return c.store.Status()
	}
	if st.ParticipationPending != nil {
		return c.store.Status()
	}
	if st.ProfilePending && st.Profile != nil {
		if _, err = c.UpdateProfile(ctx, *st.Profile); err != nil {
			return Status{}, err
		}
		if err = c.store.AckProfile(*st.Profile); err != nil {
			return Status{}, err
		}
	}
	keys := make([]string, 0, len(st.Pending))
	for key := range st.Pending {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for start := 0; start < len(keys); start += 100 {
		end := start + 100
		if end > len(keys) {
			end = len(keys)
		}
		rows := make([]social.AchievementReceipt, 0, end-start)
		for _, key := range keys[start:end] {
			rows = append(rows, st.Pending[key])
		}
		// Re-read participation before every batch to observe another host exit.
		current, err := c.store.Read()
		if err != nil {
			return Status{}, err
		}
		if current.Binding == nil || !current.Binding.Member.Participating || current.ParticipationPending != nil {
			return c.store.Status()
		}
		var ack social.SyncAck
		if err = c.request(ctx, st.Binding.BaseURL, token, http.MethodPost, "/api/v1/social/receipts", social.SyncBatch{SourceID: st.SourceID, Receipts: rows}, &ack); err != nil {
			return Status{}, err
		}
		if len(ack.Receipts) != len(rows) {
			return Status{}, fmt.Errorf("同步确认不完整")
		}
		for i := range rows {
			if rows[i] != ack.Receipts[i] {
				return Status{}, fmt.Errorf("同步确认不匹配")
			}
		}
		if err = c.store.Ack(*st.Binding, st.SourceID, ack.Receipts); err != nil {
			return Status{}, err
		}
	}
	return c.store.Status()
}
func (c *Client) Rankings(ctx context.Context, kind social.MedalKind, offset, limit int, snapshot string) (social.RankingPage, error) {
	st, token, err := c.bound()
	if err != nil {
		return social.RankingPage{}, err
	}
	query := url.Values{"kind": {string(kind)}, "offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(limit)}}
	if snapshot != "" {
		query.Set("snapshotId", snapshot)
	}
	var page social.RankingPage
	err = c.request(ctx, st.Binding.BaseURL, token, http.MethodGet, "/api/v1/social/rankings?"+query.Encode(), nil, &page)
	return page, err
}
func (c *Client) Profile(ctx context.Context, userID string) (social.PublicProfile, error) {
	if !social.ValidID(userID) {
		return social.PublicProfile{}, fmt.Errorf("成员标识无效")
	}
	st, token, err := c.bound()
	if err != nil {
		return social.PublicProfile{}, err
	}
	var profile social.PublicProfile
	err = c.request(ctx, st.Binding.BaseURL, token, http.MethodGet, "/api/v1/social/profiles/"+userID, nil, &profile)
	return profile, err
}
func (c *Client) UpdateProfile(ctx context.Context, update social.ProfileUpdate) (social.PublicProfile, error) {
	st, token, err := c.bound()
	if err != nil {
		return social.PublicProfile{}, err
	}
	update.SourceID = st.SourceID
	var profile social.PublicProfile
	err = c.request(ctx, st.Binding.BaseURL, token, http.MethodPut, "/api/v1/social/profile", update, &profile)
	return profile, err
}

func (c *Client) Showcase(ctx context.Context, update social.ProfileUpdate) (Status, error) {
	if err := c.store.QueueProfile(update); err != nil {
		return Status{}, err
	}
	return c.syncShowcase(ctx)
}
func (c *Client) ShowcaseSelection(ctx context.Context, selected []social.MedalKind) (Status, error) {
	if err := c.store.QueueSelection(selected); err != nil {
		return Status{}, err
	}
	return c.syncShowcase(ctx)
}
func (c *Client) syncShowcase(ctx context.Context) (Status, error) {
	st, err := c.store.Read()
	if err != nil {
		return Status{}, err
	}
	if st.Binding == nil || !st.Binding.Member.Participating {
		return c.store.Status()
	}
	return c.Sync(ctx)
}
