package updater

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestCheckPreferredSourceOrder(t *testing.T) {
	githubStatus := &UpdateStatus{Source: "github"}
	intranetStatus := &UpdateStatus{Source: "intranet"}
	var stages []string
	github := func(context.Context, string) (*UpdateStatus, error) { return githubStatus, nil }
	intranet := func(context.Context, string, string) (*UpdateStatus, error) {
		t.Fatal("intranet should not be called after GitHub succeeds")
		return nil, nil
	}
	status, err := checkPreferred(context.Background(), "v1.11.2", "http://127.0.0.1:8090", func(stage string) { stages = append(stages, stage) }, github, intranet)
	if err != nil || status != githubStatus || !reflect.DeepEqual(stages, []string{"github"}) {
		t.Fatalf("GitHub result = %+v, %v, stages=%v", status, err, stages)
	}
	stages = nil
	github = func(context.Context, string) (*UpdateStatus, error) { return nil, errors.New("offline") }
	intranet = func(context.Context, string, string) (*UpdateStatus, error) { return intranetStatus, nil }
	status, err = checkPreferred(context.Background(), "v1.11.2", "http://127.0.0.1:8090", func(stage string) { stages = append(stages, stage) }, github, intranet)
	if err != nil || status != intranetStatus || !reflect.DeepEqual(stages, []string{"github", "intranet"}) {
		t.Fatalf("fallback result = %+v, %v, stages=%v", status, err, stages)
	}
}

func TestCheckPreferredRespectsDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	github := func(ctx context.Context, _ string) (*UpdateStatus, error) { <-ctx.Done(); return nil, ctx.Err() }
	intranet := func(ctx context.Context, _, _ string) (*UpdateStatus, error) { return nil, ctx.Err() }
	start := time.Now()
	_, err := checkPreferred(parent, "v1.11.2", "http://127.0.0.1:8090", nil, github, intranet)
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("deadline result = %v, duration=%s", err, time.Since(start))
	}
}

func TestGitHubCheckKeepsLatestWhenHistoryTimesOut(t *testing.T) {
	var network *httptest.Server
	network = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			_ = json.NewEncoder(w).Encode(ReleaseInfo{TagName: "v1.11.3", Assets: []Asset{{Name: "opscopilot-windows.zip", BrowserDownloadURL: network.URL + "/opscopilot-windows.zip"}}})
		case "/history":
			<-r.Context().Done()
		}
	}))
	defer network.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	status, err := checkGitHub(ctx, "v1.11.2", network.URL+"/latest", network.URL+"/history")
	if err != nil || status == nil || !status.HasUpdate || status.Source != "github" {
		t.Fatalf("latest release lost after history timeout: %+v, %v", status, err)
	}
}
