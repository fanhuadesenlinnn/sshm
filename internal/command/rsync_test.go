package command

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/secret"
	"golang.org/x/crypto/ssh"
)

func TestShellCommandQuotesArguments(t *testing.T) {
	got := shellCommand([]string{"/path with space/ssh", "-o", "UserKnownHostsFile=/tmp/it's-here"})
	for _, want := range []string{"'/path with space/ssh'", "'-o'", "'UserKnownHostsFile=/tmp/it'\"'\"'s-here'"} {
		if !strings.Contains(got, want) {
			t.Fatalf("shell command %q missing %q", got, want)
		}
	}
}

func TestRsyncRemoteKeepsSpecialPathForProtectArgs(t *testing.T) {
	got := rsyncRemote(commandTestHost(), "folder with space/it's-ready")
	if got != "user@example.test:folder with space/it's-ready" {
		t.Fatalf("remote = %q", got)
	}
}

func commandTestHost() config.Host {
	return config.Host{User: "user", Host: "example.test"}
}

func TestExplicitRsyncFailsWhenCapabilityProbeCannotGuaranteeSemantics(t *testing.T) {
	_, _, used, err := tryRsyncTransfer(context.Background(), nil, nil, commandTestHost(), nil, transferOptions{
		method: "rsync", localPath: "local", remotePath: "remote",
	})
	if !used || err == nil || !strings.Contains(err.Error(), "显式 rsync 不可用") {
		t.Fatalf("used=%t err=%v", used, err)
	}
}

func TestAutoRsyncFallsBackWhenCapabilityProbeCannotGuaranteeSemantics(t *testing.T) {
	_, _, used, err := tryRsyncTransfer(context.Background(), nil, nil, commandTestHost(), nil, transferOptions{
		method: "auto", localPath: "local", remotePath: "remote",
	})
	if used || err != nil {
		t.Fatalf("auto rsync should silently fall back to SFTP: used=%t err=%v", used, err)
	}
}

func TestRsyncAvailableRejectsUnsupportedInputsBeforeExternalProbe(t *testing.T) {
	vault := secret.NewFileStore(filepath.Join(t.TempDir(), "sshm.yaml"), "master")
	host := commandTestHost()
	host.Identity = config.ManagedIdentity("personal")
	options := transferOptions{localPath: "local", remotePath: "remote"}
	if _, _, ok := rsyncAvailable(nil, host, nil, options); ok {
		t.Fatal("rsync should require an unlocked managed-key store")
	}
	host.JumpHost = "jump"
	if _, _, ok := rsyncAvailable(nil, host, vault, options); ok {
		t.Fatal("rsync should reject jump-host transfers")
	}
	host.JumpHost = ""
	host.Host = "example test"
	if _, _, ok := rsyncAvailable(nil, host, vault, options); ok {
		t.Fatal("rsync should reject unsafe endpoint syntax")
	}
	host.Host = "example.test"
	options.remotePath = "remote\npath"
	if _, _, ok := rsyncAvailable(nil, host, vault, options); ok {
		t.Fatal("rsync should reject unsafe remote paths before probing")
	}
}

func TestPrepareRsyncTransportPinsTrustAndDisablesPassword(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privateBlock, err := ssh.MarshalPrivateKey(privateKey, "managed")
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := pem.EncodeToMemory(privateBlock)
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	publicText := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey)))

	configPath := filepath.Join(t.TempDir(), "sshm.yaml")
	host := config.DefaultHost()
	host.Alias, host.User, host.Host = "server", "user", "example.test"
	host.Identity, host.HostKeyPolicy = config.ManagedIdentity("personal"), config.HostKeyPolicyStrict
	repo := config.NewRepositoryWithPath(configPath)
	if err := repo.Replace(config.DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(func(doc *config.Document) error {
		doc.ManagedKeys.Keys = []config.ManagedKey{{Name: "personal", PublicKey: publicText}}
		doc.ManagedKeys.Default = "personal"
		doc.Hosts = []config.Host{host}
		doc.HostTrust.Entries = []config.HostTrustEntry{{Host: host.Host, Port: host.Port, PublicKey: publicText}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	vault := secret.NewFileStore(configPath, "master")
	if err := vault.SetManagedKey("personal", privatePEM); err != nil {
		t.Fatal(err)
	}
	loaded, _, _, err := config.NewStoreWithPath(configPath).FindHost(host.Alias)
	if err != nil {
		t.Fatal(err)
	}
	command, cleanup, err := prepareRsyncTransport(*loaded, vault, "/usr/bin/ssh")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, want := range []string{"StrictHostKeyChecking=yes", "PasswordAuthentication=no", "PreferredAuthentications=publickey", "'-F'"} {
		if !strings.Contains(command, want) {
			t.Fatalf("transport command %q missing %q", command, want)
		}
	}
	if strings.Contains(command, string(privatePEM)) {
		t.Fatal("transport command exposed private key")
	}
	timed, timedCleanup, err := prepareRsyncTransportWithTimeout(*loaded, vault, "/usr/bin/ssh", 37*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer timedCleanup()
	if !strings.Contains(timed, "ConnectTimeout=37") {
		t.Fatalf("timed transport command = %q", timed)
	}
}
