package command

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/deploy"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ops"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/secret"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

func TestTransferOnePushesAndPullsDirectoryOverSFTP(t *testing.T) {
	remoteRoot := t.TempDir()
	addr, closeServer := startCommandTestSFTPServer(t, "secret", remoteRoot)
	defer closeServer()
	hostName, port := splitCommandTestAddress(t, addr)

	configPath := filepath.Join(t.TempDir(), "sshmd.yaml")
	store := config.NewStoreWithPath(configPath)
	initCommandTestStore(t, store)
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
	method, destination, changed, err := transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "push", localPath: localSource, remotePath: remotePath, validateChecksum: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if method != "sftp" || destination != remotePath {
		t.Fatalf("method=%q destination=%q", method, destination)
	}
	if !changed {
		t.Fatal("first push should be changed")
	}
	assertCommandTestFile(t, filepath.Join(remoteRoot, remotePath, "nested", "it's ready.txt"), "payload")

	if _, _, changed, err := transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "push", localPath: localSource, remotePath: remotePath, validateChecksum: true,
	}); err != nil || changed {
		t.Fatalf("identical push should be ok: changed=%t err=%v", changed, err)
	}

	localDestination := t.TempDir()
	method, destination, changed, err = transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "pull", remotePath: remotePath, localPath: localDestination, validateChecksum: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedDestination := filepath.Join(localDestination, filepath.Base(remotePath))
	if method != "sftp" || destination != expectedDestination {
		t.Fatalf("method=%q destination=%q want=%q", method, destination, expectedDestination)
	}
	if !changed {
		t.Fatal("first pull should be changed")
	}
	assertCommandTestFile(t, filepath.Join(expectedDestination, "nested", "it's ready.txt"), "payload")

	if err := os.WriteFile(filepath.Join(localSource, "nested", "it's ready.txt"), []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	checkResult := (&App{Store: store, ConfigPath: configPath, secretStore: vault}).operationExecutor().Push(
		context.Background(), *loaded, ops.TransferOptions{
			Src: localSource, Dest: remotePath, Method: "sftp", ValidateChecksum: true, Check: true, Diff: true,
		},
	)
	if checkResult.Err != nil || !checkResult.WouldChange ||
		!strings.Contains(checkResult.Output, "-payload") || !strings.Contains(checkResult.Output, "+updated") {
		t.Fatalf("diff check result = %+v", checkResult)
	}
	assertCommandTestFile(t, filepath.Join(remoteRoot, remotePath, "nested", "it's ready.txt"), "payload")
	if _, _, _, err := transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "push", localPath: localSource, remotePath: remotePath, overwrite: true, validateChecksum: true,
	}); err != nil {
		t.Fatal(err)
	}
	assertCommandTestFile(t, filepath.Join(remoteRoot, remotePath, "nested", "it's ready.txt"), "updated")
	if _, _, _, err := transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "pull", remotePath: remotePath, localPath: localDestination, overwrite: true, validateChecksum: true,
	}); err != nil {
		t.Fatal(err)
	}
	assertCommandTestFile(t, filepath.Join(expectedDestination, "nested", "it's ready.txt"), "updated")

	if err := os.WriteFile(filepath.Join(localSource, "nested", "it's ready.txt"), []byte("backup-new"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "push", localPath: localSource, remotePath: remotePath, backup: true, validateChecksum: true,
	}); err != nil {
		t.Fatal(err)
	}
	remoteBackups, err := filepath.Glob(filepath.Join(remoteRoot, remotePath+".bak.*"))
	if err != nil || len(remoteBackups) != 1 {
		t.Fatalf("remote backups = %v, err = %v", remoteBackups, err)
	}
	assertCommandTestFile(t, filepath.Join(remoteBackups[0], "nested", "it's ready.txt"), "updated")
	if _, _, _, err := transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "pull", remotePath: remotePath, localPath: localDestination, backup: true, validateChecksum: true,
	}); err != nil {
		t.Fatal(err)
	}
	localBackups, err := filepath.Glob(expectedDestination + ".bak.*")
	if err != nil || len(localBackups) != 1 {
		t.Fatalf("local backups = %v, err = %v", localBackups, err)
	}
	assertCommandTestFile(t, filepath.Join(localBackups[0], "nested", "it's ready.txt"), "updated")
	assertCommandTestFile(t, filepath.Join(expectedDestination, "nested", "it's ready.txt"), "backup-new")

	if _, _, _, err := transferOne(context.Background(), *loaded, vault, transferOptions{
		direction: "push", localPath: localSource, remotePath: remotePath, validateChecksum: false,
	}); err == nil || !strings.Contains(err.Error(), "--overwrite") {
		t.Fatalf("no-checksum existing target error = %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(localDestination, host.Alias, "*.sshmd-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("local temporary paths = %v, err = %v", matches, err)
	}
	matches, err = filepath.Glob(filepath.Join(remoteRoot, "*.sshmd-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("remote temporary paths = %v, err = %v", matches, err)
	}
}

func TestDeployRunnerExecutesCopyThenExecOverSharedExecutor(t *testing.T) {
	remoteRoot := t.TempDir()
	addr, closeServer := startCommandTestSFTPServer(t, "secret", remoteRoot)
	defer closeServer()
	hostName, port := splitCommandTestAddress(t, addr)

	configPath := filepath.Join(t.TempDir(), "sshmd.yaml")
	store := config.NewStoreWithPath(configPath)
	initCommandTestStore(t, store)
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
	sourceDir := t.TempDir()
	source := filepath.Join(sourceDir, "package.txt")
	if err := os.WriteFile(source, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: configPath, secretStore: vault}
	plan := &deploy.Plan{
		Name: "integration", Config: "<test>", Hosts: []config.Host{*loaded},
		Batch:          batch.Options{Parallel: 1},
		ConnectTimeout: config.Duration{Duration: 5 * time.Second},
		Timeout:        config.Duration{Duration: 5 * time.Second},
		Tasks: []deploy.Task{
			{Name: "copy", Module: "copy", BaseDir: sourceDir, ProjectRoot: sourceDir, Args: integrationArgsNode(map[string]any{"src": filepath.Base(source), "dest": "deployed/package.txt"})},
			{Name: "exec", Module: "command", Args: integrationArgsNode(map[string]any{"cmd": "verify package"})},
		},
	}
	result := (deploy.Runner{Executor: app.operationExecutor()}).Run(context.Background(), plan)
	if result.Summary.Changed != 1 || result.Summary.Failed != 0 || len(result.Hosts[0].Tasks) != 2 {
		t.Fatalf("deploy result = %+v", result)
	}
	assertCommandTestFile(t, filepath.Join(remoteRoot, "deployed", "package.txt"), "payload")
	if got := result.Hosts[0].Tasks[1].Output; got != "executed: 'verify' 'package'\n" {
		t.Fatalf("exec output = %q", got)
	}
}

func integrationArgsNode(values map[string]any) *yaml.Node {
	node := &yaml.Node{}
	data, err := yaml.Marshal(values)
	if err != nil {
		panic(err)
	}
	if err := yaml.Unmarshal(data, node); err != nil {
		panic(err)
	}
	return node
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
				if req.Type == "exec" {
					var request struct{ Command string }
					if ssh.Unmarshal(req.Payload, &request) != nil {
						_ = req.Reply(false, nil)
						return
					}
					_ = req.Reply(true, nil)
					_, _ = ch.Write([]byte("executed: " + request.Command + "\n"))
					_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
					return
				}
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
