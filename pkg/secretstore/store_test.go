package secretstore

import "testing"

type MockStore struct {
	data map[string]string
}

func NewMockStore() *MockStore {
	return &MockStore{
		data: make(map[string]string),
	}
}

func (m *MockStore) Set(service, user, password string) error {
	key := service + ":" + user
	m.data[key] = password
	return nil
}

func (m *MockStore) Get(service, user string) (string, error) {
	key := service + ":" + user
	val, ok := m.data[key]
	if !ok {
		return "", ErrNotFound
	}
	return val, nil
}

func (m *MockStore) Delete(service, user string) error {
	key := service + ":" + user
	delete(m.data, key)
	return nil
}

func TestSecretStore(t *testing.T) {
	// 验证Mock实现
	store := NewMockStore()
	
	service := "OpsCopilot"
	user := "testuser"
	pass := "secret123"

	// Test Set
	if err := store.Set(service, user, pass); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Test Get
	got, err := store.Get(service, user)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != pass {
		t.Errorf("Expected %s, got %s", pass, got)
	}

	// Test Delete
	if err := store.Delete(service, user); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Test Get after Delete
	_, err = store.Get(service, user)
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}
