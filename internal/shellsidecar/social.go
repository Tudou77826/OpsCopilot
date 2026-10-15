package shellsidecar

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"opscopilot/pkg/achievements"
	social "opscopilot/pkg/companionsocial"
	"opscopilot/pkg/config"
	"opscopilot/pkg/secretstore"
)

type SocialService struct {
	store   *achievements.Store
	client  *achievements.Client
	api     *ControlAPI
	root    string
	desktop bool
}

func newSocialService(root string, api *ControlAPI) *SocialService {
	store := achievements.NewDormant(filepath.Join(root, "achievements.json"))
	return &SocialService{store: store, client: achievements.NewClient(store, secretstore.NewKeyringStore()), api: api, root: root, desktop: api.LocalInstallation}
}
func (s *SocialService) recover() error {
	if !s.store.Enabled() {
		return nil
	}
	if s.api.Companion != nil {
		snapshot, err := s.api.Companion.inner.ReadSnapshot()
		if err != nil {
			return err
		}
		if err = s.api.Companion.queueCard(snapshot); err != nil {
			return err
		}
	}
	sources := &achievements.Sources{Scripts: s.api.Scripts.mgr.StorageDirectory(), Recordings: s.api.Scripts.rec.StorageDirectory(), Cases: filepath.Join(s.root, "cases"), Knowledge: filepath.Join(s.root, "knowledge"),
		QuickCommands: func() (map[string]bool, error) {
			commands, err := s.api.QuickCmds.ListChecked()
			if err != nil {
				return nil, err
			}
			items := map[string]bool{}
			for _, cmd := range commands {
				items[cmd.ID] = strings.TrimSpace(cmd.Content) != ""
			}
			return items, nil
		},
	}
	if s.desktop {
		sources.Cases = ""
		m := config.NewManagerWithDir(s.root)
		m.SetReadOnly(true)
		if err := m.Load(); err != nil {
			return err
		}
		sources.Knowledge = m.Config.Docs.Dir
		if sources.Knowledge == "" {
			sources.Knowledge = filepath.Join(s.root, "docs")
			if _, err := os.Stat(sources.Knowledge); os.IsNotExist(err) {
				sources.Knowledge = filepath.Join(s.root, "knowledge")
			}
		}
	}
	return s.store.Repair(sources)
}

func (s *SocialService) day() {
	_ = s.store.Record(social.MedalActiveDay, time.Now().UTC().Format("2006-01-02"), true)
}
func (a *ControlAPI) dispatchSocial(ctx context.Context, req *rpcRequest) (any, *rpcError) {
	if a.Social == nil || !a.Social.store.Enabled() {
		return nil, notEnabled()
	}
	s := a.Social
	if err := s.recover(); err != nil {
		return nil, srvErr(err)
	}
	var result any
	var err error
	switch req.Method {
	case "shell.social.status":
		result, err = s.store.Status()
	case "shell.social.catalog":
		result = social.MedalCatalog()
	case "shell.social.connect":
		var p struct {
			BaseURL    string `json:"baseUrl"`
			Credential string `json:"credential"`
			ClaimLocal bool   `json:"claimLocal"`
		}
		if e := json.Unmarshal(req.Params, &p); e != nil {
			return nil, badParams(e)
		}
		result, err = s.client.Connect(ctx, p.BaseURL, p.Credential, p.ClaimLocal)
	case "shell.social.participation":
		var p struct {
			Enabled *bool `json:"enabled"`
		}
		if e := json.Unmarshal(req.Params, &p); e != nil || p.Enabled == nil {
			return nil, badParams(fmt.Errorf("需要参与选择"))
		}
		err = s.client.Participation(ctx, *p.Enabled)
		result = map[string]any{}
	case "shell.social.sync":
		result, err = s.client.Sync(ctx)
	case "shell.social.rankings":
		var p struct {
			Kind       social.MedalKind `json:"kind"`
			Offset     int              `json:"offset"`
			Limit      int              `json:"limit"`
			SnapshotID string           `json:"snapshotId"`
		}
		if e := json.Unmarshal(req.Params, &p); e != nil {
			return nil, badParams(e)
		}
		result, err = s.client.Rankings(ctx, p.Kind, p.Offset, p.Limit, p.SnapshotID)
	case "shell.social.profile":
		var p struct {
			UserID string `json:"userId"`
		}
		if e := json.Unmarshal(req.Params, &p); e != nil {
			return nil, badParams(e)
		}
		result, err = s.client.Profile(ctx, p.UserID)
	case "shell.social.showcase":
		var p struct {
			Selected []social.MedalKind `json:"selected"`
		}
		if e := json.Unmarshal(req.Params, &p); e != nil {
			return nil, badParams(e)
		}
		result, err = s.client.ShowcaseSelection(ctx, p.Selected)
	default:
		return nil, &rpcError{Code: -32601, Message: "unknown social method"}
	}
	if err != nil {
		return nil, srvErr(err)
	}
	return result, nil
}
