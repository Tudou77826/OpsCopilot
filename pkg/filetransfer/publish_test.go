package filetransfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadPublicationNeverClobbersConcurrentDestination(t *testing.T) {
	for _, scenario := range []string{"new", "exists", "race", "cancel", "failure"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "report.txt")
			if scenario == "exists" {
				if err := os.WriteFile(dest, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := DownloadNewFile(ctx, dest, func(stage string) (TransferResult, error) {
				if err := os.WriteFile(stage, []byte("download"), 0600); err != nil {
					return TransferResult{}, err
				}
				if scenario == "race" {
					if err := os.WriteFile(dest, []byte("keep"), 0600); err != nil {
						return TransferResult{}, err
					}
				}
				if scenario == "cancel" {
					cancel()
				}
				if scenario == "failure" {
					return TransferResult{}, errors.New("transfer failed")
				}
				return TransferResult{}, nil
			})
			if (err == nil) != (scenario == "new") {
				t.Fatalf("unexpected publication result: %v", err)
			}
			data, readErr := os.ReadFile(dest)
			if scenario == "exists" || scenario == "race" {
				if readErr != nil || string(data) != "keep" {
					t.Fatal("overwrote user file", readErr)
				}
			}
			if scenario == "new" && (readErr != nil || string(data) != "download") {
				t.Fatal("missing download", readErr)
			}
			if scenario == "cancel" || scenario == "failure" {
				if !os.IsNotExist(readErr) {
					t.Fatal("published failed/cancelled transfer")
				}
			}
			stages, _ := filepath.Glob(filepath.Join(dir, ".opscopilot-download-*"))
			if len(stages) != 0 {
				t.Fatal("staging files leaked")
			}
		})
	}
}
