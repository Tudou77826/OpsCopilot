package patchstore

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPendingStoreSaveListDelete(t *testing.T) {
	store := NewPendingStore(filepath.Join(t.TempDir(), "pending"))
	patch := Patch{
		ID:        "abc12345",
		Service:   "Order Service",
		Module:    "订单处理",
		Author:    "alice",
		Timestamp: time.Date(2026, 5, 18, 11, 22, 33, 0, time.FixedZone("UTC+8", 8*3600)),
		Content:   "## 场景：Redis 连接超时\n\n排查记录",
	}

	if err := store.Save(patch); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	patches, err := store.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(patches))
	}
	if patches[0].ID != patch.ID {
		t.Fatalf("unexpected patch id: got %s want %s", patches[0].ID, patch.ID)
	}
	if !patches[0].Timestamp.Equal(patch.Timestamp) {
		t.Fatalf("unexpected patch timestamp: got %v want %v", patches[0].Timestamp, patch.Timestamp)
	}

	if err := store.Delete(patch); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	patches, err = store.List()
	if err != nil {
		t.Fatalf("List after delete error: %v", err)
	}
	if len(patches) != 0 {
		t.Fatalf("expected 0 patches after delete, got %d", len(patches))
	}
}
