package sshx

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/secret"
	"golang.org/x/crypto/ssh"
)

func TestDialContextUsesStoredTrustAndPassword(t *testing.T) {
	addr, closeServer := startTestSSHServer(t, "secret", false)
	defer closeServer()
	hostName, port := splitTestAddress(t, addr)
	path := filepath.Join(t.TempDir(), "sshmd.yaml")
	store := config.NewStoreWithPath(path)
	initSSHXTestStore(t, store)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host, host.Port = "target", "test", hostName, port
	host.PasswordRef = host.ID
	host.HostKeyPolicy = config.HostKeyPolicyAcceptNew
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	vault := secret.NewFileStore(path, "master")
	if err := vault.SetPassword(host.ID, "secret"); err != nil {
		t.Fatal(err)
	}
	loaded, _, _, err := store.FindHost("target")
	if err != nil {
		t.Fatal(err)
	}
	client, _, err := DialContext(context.Background(), *loaded, vault)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	output, err := session.CombinedOutput("echo ok")
	if err != nil || string(output) != "ok\n" {
		t.Fatalf("output = %q, err = %v", output, err)
	}
	exitSession, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	_, err = exitSession.CombinedOutput("exit 127")
	status, ok := remoteSessionExitStatus(err)
	if !ok || status != 127 {
		t.Fatalf("remote exit status = %d, recognized = %t, err = %v", status, ok, err)
	}
	client.Close()
	doc, err := config.NewRepositoryWithPath(path).Load()
	if err != nil || len(doc.HostTrust.Entries) != 1 {
		t.Fatalf("host trust = %+v, %v", doc.HostTrust.Entries, err)
	}
}

func TestDialContextUsesPlaintextPasswordWithoutVault(t *testing.T) {
	addr, closeServer := startTestSSHServer(t, "secret", false)
	defer closeServer()
	hostName, port := splitTestAddress(t, addr)
	path := filepath.Join(t.TempDir(), "sshmd.yaml")
	store := config.NewStoreWithPath(path)
	initSSHXTestStore(t, store)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host, host.Port = "target", "test", hostName, port
	host.Password = "secret"
	host.HostKeyPolicy = config.HostKeyPolicyAcceptNew
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	loaded, _, _, err := store.FindHost("target")
	if err != nil {
		t.Fatal(err)
	}

	// Plaintext password must work with an initialized config but no vault
	// entries at all.
	vault := secret.NewFileStore(path, "master")
	client, _, err := DialContext(context.Background(), *loaded, vault)
	if err != nil {
		t.Fatalf("明文密码连接失败: %v", err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	output, err := session.CombinedOutput("echo ok")
	if err != nil || string(output) != "ok\n" {
		t.Fatalf("output = %q, err = %v", output, err)
	}
}

func TestClientConfigUsesExplicitConnectionTimeout(t *testing.T) {
	host := config.DefaultHost()
	host.Alias, host.User, host.Host = "server", "test", "127.0.0.1"
	host.PasswordRef = host.ID
	path := filepath.Join(t.TempDir(), "sshmd.yaml")
	if err := config.NewRepositoryWithPath(path).Replace(config.DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	vault := secret.NewFileStore(path, "master")
	if err := vault.SetPassword(host.ID, "secret"); err != nil {
		t.Fatal(err)
	}
	timeout := 37 * time.Second
	sshConfig, _, err := clientConfigWithTimeout(host, vault, timeout)
	if err != nil {
		t.Fatal(err)
	}
	if sshConfig.Timeout != timeout {
		t.Fatalf("timeout = %s", sshConfig.Timeout)
	}
}

func TestClientConfigClassifiesMissingCredentials(t *testing.T) {
	host := config.DefaultHost()
	host.Alias, host.User, host.Host = "server", "test", "127.0.0.1"
	host.HostKeyPolicy = config.HostKeyPolicyInsecure
	_, _, err := clientConfigWithTimeout(host, nil, time.Second)
	if err == nil {
		t.Fatal("missing credentials should fail")
	}
	if stage := operation.StageOf(err, operation.StageAuth); stage != operation.StageCredential {
		t.Fatalf("stage = %q, error = %v", stage, err)
	}
}

func TestClientConfigPreservesManagedKeyVaultStage(t *testing.T) {
	host := config.DefaultHost()
	host.Alias, host.User, host.Host = "server", "test", "127.0.0.1"
	host.Auth = "key"
	host.Identity = config.ManagedIdentity("personal")
	host.HostKeyPolicy = config.HostKeyPolicyInsecure
	_, _, err := clientConfigWithTimeout(host, nil, time.Second)
	if err == nil {
		t.Fatal("locked managed key vault should fail")
	}
	if stage := operation.StageOf(err, operation.StageAuth); stage != operation.StageVault {
		t.Fatalf("stage = %q, error = %v", stage, err)
	}
}

func TestDialContextUsesSingleJumpHost(t *testing.T) {
	targetAddr, closeTarget := startTestSSHServer(t, "target-pass", false)
	defer closeTarget()
	jumpAddr, closeJump := startTestSSHServer(t, "jump-pass", true)
	defer closeJump()
	targetHost, targetPort := splitTestAddress(t, targetAddr)
	jumpHost, jumpPort := splitTestAddress(t, jumpAddr)

	path := filepath.Join(t.TempDir(), "sshmd.yaml")
	store := config.NewStoreWithPath(path)
	initSSHXTestStore(t, store)
	jump := config.DefaultHost()
	jump.Alias, jump.User, jump.Host, jump.Port = "jump", "test", jumpHost, jumpPort
	jump.PasswordRef, jump.HostKeyPolicy = jump.ID, config.HostKeyPolicyAcceptNew
	target := config.DefaultHost()
	target.Alias, target.User, target.Host, target.Port = "target", "test", targetHost, targetPort
	target.PasswordRef, target.HostKeyPolicy, target.JumpHost = target.ID, config.HostKeyPolicyAcceptNew, jump.Alias
	if err := store.Add(jump); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(target); err != nil {
		t.Fatal(err)
	}
	vault := secret.NewFileStore(path, "master")
	if err := vault.SetPassword(jump.ID, "jump-pass"); err != nil {
		t.Fatal(err)
	}
	if err := vault.SetPassword(target.ID, "target-pass"); err != nil {
		t.Fatal(err)
	}
	loaded, _, _, err := store.FindHost("target")
	if err != nil {
		t.Fatal(err)
	}
	client, _, err := DialContext(context.Background(), *loaded, vault)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	output, err := session.CombinedOutput("echo ok")
	client.Close()
	if err != nil || string(output) != "ok\n" {
		t.Fatalf("output = %q, err = %v", output, err)
	}
}

func TestDialContextClassifiesDNSFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshmd.yaml")
	store := config.NewStoreWithPath(path)
	initSSHXTestStore(t, store)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host = "missing", "test", "missing.invalid"
	host.PasswordRef, host.HostKeyPolicy = host.ID, config.HostKeyPolicyAcceptNew
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	vault := secret.NewFileStore(path, "master")
	if err := vault.SetPassword(host.ID, "secret"); err != nil {
		t.Fatal(err)
	}
	loaded, _, _, err := store.FindHost(host.Alias)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = DialContext(context.Background(), *loaded, vault)
	if err == nil {
		t.Fatal("missing host should fail")
	}
	if stage := operation.StageOf(err, operation.StageNetwork); stage != operation.StageResolve {
		t.Fatalf("stage = %q, error = %v", stage, err)
	}
}

func TestDialContextCancelsStalledHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	hostName, port := splitTestAddress(t, listener.Addr().String())
	host := config.Host{Alias: "stalled", User: "test", Host: hostName, Port: port}
	sshConfig := &ssh.ClientConfig{
		User:            host.User,
		Auth:            []ssh.AuthMethod{ssh.Password("secret")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, _, err := dialDirectWithConfig(ctx, host, sshConfig, "密码"); err == nil {
		t.Fatal("stalled handshake should be canceled")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("stalled handshake cancellation took %s", elapsed)
	}
	select {
	case conn := <-accepted:
		_ = conn.Close()
	default:
	}
}

func startTestSSHServer(t *testing.T, password string, forward bool) (string, func()) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if string(pass) != password {
				return nil, fmt.Errorf("bad password")
			}
			return nil, nil
		},
	}
	serverConfig.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveTestSSHConnection(conn, serverConfig, forward)
		}
	}()
	return listener.Addr().String(), func() { _ = listener.Close() }
}

func serveTestSSHConnection(conn net.Conn, serverConfig *ssh.ServerConfig, forward bool) {
	serverConn, channels, requests, err := ssh.NewServerConn(conn, serverConfig)
	if err != nil {
		_ = conn.Close()
		return
	}
	go ssh.DiscardRequests(requests)
	go func() {
		defer serverConn.Close()
		for channel := range channels {
			switch channel.ChannelType() {
			case "session":
				ch, reqs, err := channel.Accept()
				if err != nil {
					continue
				}
				go func() {
					defer ch.Close()
					for req := range reqs {
						if req.Type == "exec" {
							_ = req.Reply(true, nil)
							var payload struct{ Command string }
							_ = ssh.Unmarshal(req.Payload, &payload)
							status := uint32(0)
							if payload.Command == "exit 127" {
								status = 127
							} else {
								_, _ = ch.Write([]byte("ok\n"))
							}
							_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
							return
						}
						_ = req.Reply(false, nil)
					}
				}()
			case "direct-tcpip":
				if !forward {
					_ = channel.Reject(ssh.Prohibited, "forwarding disabled")
					continue
				}
				var target struct {
					Host       string
					Port       uint32
					OriginHost string
					OriginPort uint32
				}
				if err := ssh.Unmarshal(channel.ExtraData(), &target); err != nil {
					_ = channel.Reject(ssh.ConnectionFailed, err.Error())
					continue
				}
				remote, err := net.Dial("tcp", net.JoinHostPort(target.Host, fmt.Sprintf("%d", target.Port)))
				if err != nil {
					_ = channel.Reject(ssh.ConnectionFailed, err.Error())
					continue
				}
				ch, reqs, err := channel.Accept()
				if err != nil {
					remote.Close()
					continue
				}
				go ssh.DiscardRequests(reqs)
				go func() {
					defer ch.Close()
					defer remote.Close()
					go io.Copy(ch, remote)
					_, _ = io.Copy(remote, ch)
				}()
			default:
				_ = channel.Reject(ssh.UnknownChannelType, "unsupported")
			}
		}
	}()
}

func splitTestAddress(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	return host, port
}
