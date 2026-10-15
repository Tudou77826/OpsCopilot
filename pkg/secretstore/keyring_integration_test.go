package secretstore

import (
	"github.com/google/uuid"
	"os"
	"testing"
)

func TestKeyringIsolatedRoundTrip(t *testing.T) {
	if os.Getenv("OPS_SOCIAL_KEYRING_TEST") != "1" {
		t.Skip("explicit OS credential-store integration test")
	}
	store := NewKeyringStore()
	service := "OpsCopilot-Social-Test"
	user := uuid.NewString()
	secret := uuid.NewString()
	if err := store.Set(service, user, secret); err != nil {
		t.Fatal("Keyring write failed", err)
	}
	defer func() {
		if err := store.Delete(service, user); err != nil {
			t.Error("Keyring cleanup failed", err)
		}
	}()
	saved, err := NewKeyringStore().Get(service, user)
	if err != nil || saved != secret {
		t.Fatal("Keyring cross-instance round trip failed", err)
	}
}
