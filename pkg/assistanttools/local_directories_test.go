package assistanttools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistantlocal"
)

func TestLocalDirectoryAdditionRequiresExactConsentAndStopRevokesIt(t *testing.T) {
	for _, outcome := range []string{"allow", "reject", "stop", "switch", "deleted"} {
		t.Run(outcome, func(t *testing.T) {
			coordinator := &assistantcontrol.Coordinator{}
			turn, _ := coordinator.Begin(context.Background(), assistantcontrol.Target{}, nil)
			defer turn.Finish("completed", "")
			store := &assistantlocal.Projects{}
			created, err := store.Create("A项目")
			if err != nil {
				t.Fatal(err)
			}
			projectID := created.ActiveID
			tools := &LocalDirectoryTools{Turn: turn, Projects: store}
			paths := []string{t.TempDir(), t.TempDir()}
			args, _ := json.Marshal(map[string]any{"project_id": projectID, "paths": paths, "purpose": "保存下载包并查看项目文件"})
			done := make(chan error, 1)
			go func() { _, err := tools.Call(turn.Context(), "local_add_directories", string(args)); done <- err }()
			var prompt *assistantcontrol.Prompt
			for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
				prompt = coordinator.Snapshot().Prompt
				if prompt != nil {
					break
				}
				time.Sleep(time.Millisecond)
			}
			before, _ := store.Snapshot()
			if prompt == nil || len(before.Projects[0].Directories) != 0 || len(prompt.Details) != 2 || prompt.Title != "将本地目录加入项目「A项目」" {
				t.Fatal("missing confirmation or premature add", prompt)
			}
			canonical, _ := assistantlocal.Normalize(paths)
			if prompt.Details[0].Value != canonical[0] || prompt.Details[1].Value != canonical[1] {
				t.Fatal("wrong approval paths", prompt.Details)
			}
			if outcome == "switch" || outcome == "deleted" {
				if _, err = store.Create("B项目"); err != nil {
					t.Fatal(err)
				}
			}
			if outcome == "deleted" {
				if _, err = store.Delete(projectID); err != nil {
					t.Fatal(err)
				}
			}
			selected := outcome
			if selected == "switch" || selected == "deleted" {
				selected = "allow"
			}
			if outcome == "stop" {
				_ = coordinator.Stop(turn.ID())
				if err := coordinator.Respond(turn.ID(), prompt.ID, assistantcontrol.Answer{Selected: []string{"allow"}}); !errors.Is(err, assistantcontrol.ErrStale) {
					t.Fatal(err)
				}
			} else if err := coordinator.Respond(turn.ID(), prompt.ID, assistantcontrol.Answer{Selected: []string{selected}}); err != nil {
				t.Fatal(err)
			}
			err = <-done
			after, _ := store.Snapshot()
			if outcome == "allow" || outcome == "switch" {
				if err != nil || len(after.Projects[0].Directories) != 2 {
					t.Fatal(after, err)
				}
				if outcome == "switch" && len(after.Projects[1].Directories) != 0 {
					t.Fatal("approval redirected to a different project")
				}
			} else if err == nil || len(after.Projects[0].Directories) != 0 {
				t.Fatal("unapproved change", after, err)
			}
		})
	}
}
