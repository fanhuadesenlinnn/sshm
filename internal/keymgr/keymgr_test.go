package keymgr

import (
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateAndParseManagedKey(t *testing.T) {
	privateKey, publicKey, err := GenerateManagedKey("personal")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(publicKey, "sshmd:personal") {
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
