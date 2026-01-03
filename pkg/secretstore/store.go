package secretstore

import (
	"errors"

	"github.com/zalando/go-keyring"
)

var ErrNotFound = errors.New("secret not found")

type SecretStore interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type KeyringStore struct{}

func NewKeyringStore() *KeyringStore {
	return &KeyringStore{}
}

func (k *KeyringStore) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (k *KeyringStore) Get(service, user string) (string, error) {
	val, err := keyring.Get(service, user)
	if err == keyring.ErrNotFound {
		return "", ErrNotFound
	}
	return val, err
}

func (k *KeyringStore) Delete(service, user string) error {
	return keyring.Delete(service, user)
}
