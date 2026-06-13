package command

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v4/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/secret"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestTransferOnePushesAndPullsDirectoryOverSFTP(t *testing.T) {
	remoteRoot := t.TempDir()
	addr, closeServer := startCommandTestSFTPServer(t, "secret", remoteRoot)
	defer closeServer()
	hostName, port := splitCommandTestAddress(t, addr)

	configPath := filepath.Join(t.TempDir(), "sshm.yaml")
	store := config.NewStoreWithPath(configPath)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host, host.Port = "target", "test", hostName, port
	host.PasswordRef, host.HostKeyPolicy = host.ID, config.HostKeyPolicyAcceptNew
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	vault := secret.NewFileStore(configPath, "master")
	if err := vault.SetPassword(host.ID, "secret"); err != nil {
		t.Fatal(err)
	}
	loaded, _, _, err := store.FindHost(host.Alias)
	if err != nil {
		t.Fatal(err)
	}

	localSource := filepath.Join(t.TempDir(), "source dir")
	if err := os.MkdirAll(filepath.Join(localSource, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localSource, "nested", "it's ready.txt"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	remotePath := "remote target 'one'"
	method, destination, err := transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "push", localPath: localSource, remotePath: remotePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if method != "sftp" || destination != remotePath {
		t.Fatalf("method=%q destination=%q", method, destination)
	}
	assertCommandTestFile(t, filepath.Join(remoteRoot, remotePath, "nested", "it's ready.txt"), "payload")

	if _, _, err := transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "push", localPath: localSource, remotePath: remotePath,
	}); err == nil {
		t.Fatal("push should refuse an existing destination without --overwrite")
	}

	localDestination := t.TempDir()
	method, destination, err = transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "pull", remotePath: remotePath, localPath: localDestination,
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedDestination := filepath.Join(localDestination, host.Alias, filepath.Base(remotePath))
	if method != "sftp" || destination != expectedDestination {
		t.Fatalf("method=%q destination=%q want=%q", method, destination, expectedDestination)
	}
	assertCommandTestFile(t, filepath.Join(expectedDestination, "nested", "it's ready.txt"), "payload")

	if err := os.WriteFile(filepath.Join(localSource, "nested", "it's ready.txt"), []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "push", localPath: localSource, remotePath: remotePath, overwrite: true,
	}); err != nil {
		t.Fatal(err)
	}
	assertCommandTestFile(t, filepath.Join(remoteRoot, remotePath, "nested", "it's ready.txt"), "updated")
	if _, _, err := transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "pull", remotePath: remotePath, localPath: localDestination, overwrite: true,
	}); err != nil {
		t.Fatal(err)
	}
	assertCommandTestFile(t, filepath.Join(expectedDestination, "nested", "it's ready.txt"), "updated")

	matches, err := filepath.Glob(filepath.Join(localDestination, host.Alias, "*.sshm-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("local temporary paths = %v, err = %v", matches, err)
	}
	matches, err = filepath.Glob(filepath.Join(remoteRoot, "*.sshm-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("remote temporary paths = %v, err = %v", matches, err)
	}
}

func startCommandTestSFTPServer(t *testing.T, password, workDir string) (string, func()) {
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
			go serveCommandTestSFTPConnection(conn, serverConfig, workDir)
		}
	}()
	return listener.Addr().String(), func() { _ = listener.Close() }
}

func serveCommandTestSFTPConnection(conn net.Conn, serverConfig *ssh.ServerConfig, workDir string) {
	serverConn, channels, requests, err := ssh.NewServerConn(conn, serverConfig)
	if err != nil {
		_ = conn.Close()
		return
	}
	go ssh.DiscardRequests(requests)
	defer serverConn.Close()
	for channel := range channels {
		if channel.ChannelType() != "session" {
			_ = channel.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		ch, reqs, err := channel.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer ch.Close()
			for req := range reqs {
				var subsystem struct{ Name string }
				if req.Type != "subsystem" || ssh.Unmarshal(req.Payload, &subsystem) != nil || subsystem.Name != "sftp" {
					_ = req.Reply(false, nil)
					continue
				}
				_ = req.Reply(true, nil)
				server, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(workDir))
				if err == nil {
					_ = server.Serve()
					_ = server.Close()
				}
				return
			}
		}()
	}
}

func splitCommandTestAddress(t *testing.T, addr string) (string, int) {
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

func assertCommandTestFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}
