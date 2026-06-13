package keymgr

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"golang.org/x/crypto/ssh"
)

func TestImportKeyValidatesAndRefusesOverwrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SSHM_HOME", home)

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(privateKey, "test")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(source, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}

	relative, err := ImportKey("server", source)
	if err != nil {
		t.Fatal(err)
	}
	if !IsManagedKey(relative) {
		t.Fatalf("imported key %q should be managed", relative)
	}
	if _, err := ImportKey("server", source); err == nil {
		t.Fatal("ImportKey() should refuse overwrite")
	}
	if err := RemoveManagedKey(relative); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(config.ExpandPath(relative)); !os.IsNotExist(err) {
		t.Fatalf("managed key still exists: %v", err)
	}
}

func TestImportKeyRejectsInvalidPrivateKey(t *testing.T) {
	t.Setenv("SSHM_HOME", t.TempDir())
	source := filepath.Join(t.TempDir(), "not-a-key")
	if err := os.WriteFile(source, []byte("not a private key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportKey("server", source); err == nil {
		t.Fatal("ImportKey() should reject invalid private key")
	}
}

func TestRemoveManagedKeyRejectsExternalPath(t *testing.T) {
	t.Setenv("SSHM_HOME", t.TempDir())
	if err := RemoveManagedKey(filepath.Join(t.TempDir(), "external")); err == nil {
		t.Fatal("RemoveManagedKey() should reject external path")
	}
}

func TestGenerateAndParseManagedKey(t *testing.T) {
	privateKey, publicKey, err := GenerateManagedKey("personal")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(publicKey, "sshm:personal") {
		t.Fatalf("public key comment missing: %q", publicKey)
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		t.Fatalf("generated private key is invalid: %v", err)
	}
	parsed, parsedPublic, err := ParseManagedKey(privateKey, nil, "imported")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ssh.ParsePrivateKey(parsed); err != nil {
		t.Fatalf("normalized private key is invalid: %v", err)
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(parsedPublic))
	if err != nil {
		t.Fatal(err)
	}
	if ssh.FingerprintSHA256(signer.PublicKey()) != ssh.FingerprintSHA256(key) {
		t.Fatal("public key changed during import")
	}
}
