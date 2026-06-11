package secret

import (
	"errors"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEncryptedFileYAMLRoundTrip(t *testing.T) {
	encrypted, err := Encrypt("password:server:secret", "master-password", nil)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	data, err := yaml.Marshal(encrypted)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}

	var decoded EncryptedFile
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if decoded.Scrypt.N != defaultN || decoded.Scrypt.R != defaultR ||
		decoded.Scrypt.P != defaultP || decoded.Scrypt.KeyLen != defaultKeyLen {
		t.Fatalf("decoded scrypt config = %+v, want defaults", decoded.Scrypt)
	}

	plain, err := Decrypt(&decoded, "master-password")
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if plain != "password:server:secret" {
		t.Fatalf("Decrypt() = %q, want %q", plain, "password:server:secret")
	}
}

func TestEncryptedFileParsesExistingNestedScryptConfig(t *testing.T) {
	data := []byte(`version: 1
kdf: scrypt
cipher: aes-256-gcm
scrypt:
  n: 32768
  r: 8
  p: 1
  key_len: 32
salt: unused
nonce: unused
ciphertext: unused
`)

	var encrypted EncryptedFile
	if err := yaml.Unmarshal(data, &encrypted); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if encrypted.Scrypt.N != defaultN || encrypted.Scrypt.R != defaultR ||
		encrypted.Scrypt.P != defaultP || encrypted.Scrypt.KeyLen != defaultKeyLen {
		t.Fatalf("decoded scrypt config = %+v, want defaults", encrypted.Scrypt)
	}
}

func TestDecryptRejectsIncorrectPassphrase(t *testing.T) {
	encrypted, err := Encrypt("secret", "correct-password", nil)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	_, err = Decrypt(encrypted, "wrong-password")
	if !errors.Is(err, ErrIncorrectPassphrase) {
		t.Fatalf("Decrypt() error = %v, want ErrIncorrectPassphrase", err)
	}
}

func TestDecryptRejectsInvalidScryptConfig(t *testing.T) {
	encrypted, err := Encrypt("secret", "master-password", nil)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	encrypted.Scrypt.N = 0

	_, err = Decrypt(encrypted, "master-password")
	if err == nil {
		t.Fatalf("Decrypt() error = %v, want invalid scrypt config error", err)
	}
}
