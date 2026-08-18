package sessionshare

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	payload := SecretsPayload{
		Password:     "p@ssw0rd-中文",
		RootPassword: "rootpw",
		Bastion: &BastionSecrets{
			Host: "10.0.0.254", Port: 22, User: "jump", Password: "jp",
		},
	}

	enc, err := EncryptSecrets(payload, "团队密钥")
	if err != nil {
		t.Fatalf("EncryptSecrets error: %v", err)
	}
	if !strings.HasPrefix(enc, "v1:") {
		t.Errorf("ciphertext should carry v1 prefix, got %q", enc[:10])
	}
	if strings.Contains(enc, "p@ssw0rd") {
		t.Errorf("ciphertext must not contain plaintext password")
	}

	dec, err := DecryptSecrets(enc, "团队密钥")
	if err != nil {
		t.Fatalf("DecryptSecrets error: %v", err)
	}
	if dec.Password != payload.Password || dec.RootPassword != payload.RootPassword {
		t.Errorf("roundtrip mismatch: %+v", dec)
	}
	if dec.Bastion == nil || dec.Bastion.Host != payload.Bastion.Host || dec.Bastion.Password != payload.Bastion.Password {
		t.Errorf("bastion roundtrip mismatch: %+v", dec.Bastion)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	enc, err := EncryptSecrets(SecretsPayload{Password: "secret"}, "correct-key")
	if err != nil {
		t.Fatalf("EncryptSecrets error: %v", err)
	}

	if _, err := DecryptSecrets(enc, "wrong-key"); err == nil {
		t.Fatal("expected error with wrong key")
	} else if !strings.Contains(err.Error(), "共享密钥可能不正确") {
		t.Errorf("error should hint wrong key, got: %v", err)
	}
}

func TestDecryptTampered(t *testing.T) {
	enc, err := EncryptSecrets(SecretsPayload{Password: "secret"}, "k")
	if err != nil {
		t.Fatalf("EncryptSecrets error: %v", err)
	}

	// 篡改密文末尾一个字符
	tampered := enc[:len(enc)-2] + "A="
	if _, err := DecryptSecrets(tampered, "k"); err == nil {
		t.Fatal("expected error with tampered ciphertext")
	}
}

func TestEncryptEmptyPassphrase(t *testing.T) {
	if _, err := EncryptSecrets(SecretsPayload{Password: "x"}, ""); err == nil {
		t.Fatal("expected error with empty passphrase")
	}
	if _, err := EncryptSecrets(SecretsPayload{Password: "x"}, "   "); err == nil {
		t.Fatal("expected error with whitespace passphrase")
	}
}

func TestDecryptEmptyEnc(t *testing.T) {
	dec, err := DecryptSecrets("", "any")
	if err != nil {
		t.Fatalf("empty enc should be valid (no secrets), got: %v", err)
	}
	if dec.Password != "" || dec.Bastion != nil {
		t.Errorf("empty enc should yield zero payload, got %+v", dec)
	}
}

func TestCiphertextNonDeterministic(t *testing.T) {
	// 每次加密使用随机 salt+nonce，相同明文密文应不同
	p := SecretsPayload{Password: "same"}
	enc1, _ := EncryptSecrets(p, "k")
	enc2, _ := EncryptSecrets(p, "k")
	if enc1 == enc2 {
		t.Error("ciphertexts of same plaintext must differ (random salt/nonce)")
	}
}
