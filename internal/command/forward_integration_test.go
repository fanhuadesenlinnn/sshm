package command

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

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/sshx"
	"golang.org/x/crypto/ssh"
)

func TestRunLocalForwardCarriesTraffic(t *testing.T) {
	echoListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoListener.Close()
	go func() {
		for {
			conn, err := echoListener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()

	sshAddr, closeSSH := startCommandTestForwardServer(t, "secret")
	defer closeSSH()
	hostName, port := splitCommandTestAddress(t, sshAddr)
	configPath := filepath.Join(t.TempDir(), "sshm.yaml")
	store := config.NewStoreWithPath(configPath)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host, host.Port = "forwarder", "test", hostName, port
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
	client, _, err := sshx.DialContext(context.Background(), *loaded, vault)
	if err != nil {
		t.Fatal(err)
	}

	localListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runLocalForward(ctx, client, localListener, echoListener.Addr().String())
	}()

	conn, err := net.DialTimeout("tcp", localListener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("through-ssh")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, len("through-ssh"))
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if string(reply) != "through-ssh" {
		t.Fatalf("reply = %q", reply)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("forward did not stop after cancellation")
	}
}

func startCommandTestForwardServer(t *testing.T, password string) (string, func()) {
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
			go serveCommandTestForwardConnection(conn, serverConfig)
		}
	}()
	return listener.Addr().String(), func() { _ = listener.Close() }
}

func serveCommandTestForwardConnection(conn net.Conn, serverConfig *ssh.ServerConfig) {
	serverConn, channels, requests, err := ssh.NewServerConn(conn, serverConfig)
	if err != nil {
		_ = conn.Close()
		return
	}
	go ssh.DiscardRequests(requests)
	defer serverConn.Close()
	for channel := range channels {
		if channel.ChannelType() != "direct-tcpip" {
			_ = channel.Reject(ssh.UnknownChannelType, "unsupported")
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
			_ = remote.Close()
			continue
		}
		go ssh.DiscardRequests(reqs)
		go func() {
			defer ch.Close()
			defer remote.Close()
			go io.Copy(ch, remote)
			_, _ = io.Copy(remote, ch)
		}()
	}
}
