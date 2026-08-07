package command

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/sshx"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
	"golang.org/x/crypto/ssh"
)

func (app *App) cmdForward(args []string) error {
	if len(args) < 3 || len(args)%2 == 0 {
		return fmt.Errorf("用法: sshm forward <别名|ID> <本地监听> <远程目标> [<本地监听> <远程目标> ...]\n示例: sshm forward prod 127.0.0.1:8080 127.0.0.1:80 127.0.0.1:8443 127.0.0.1:443")
	}
	pairs := make([]forwardPair, 0, (len(args)-1)/2)
	for i := 1; i < len(args); i += 2 {
		local, remote := args[i], args[i+1]
		if _, _, err := net.SplitHostPort(local); err != nil {
			return fmt.Errorf("无效本地监听地址 %q: %w", local, err)
		}
		if _, _, err := net.SplitHostPort(remote); err != nil {
			return fmt.Errorf("无效远程目标 %q: %w", remote, err)
		}
		pairs = append(pairs, forwardPair{local: local, remote: remote})
	}
	host, _, _, err := app.findHost(args[0])
	if err != nil {
		return err
	}
	store := app.getSecretStoreForHost(host)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	client, _, err := sshx.DialContext(ctx, *host, store)
	if err != nil {
		return err
	}
	defer client.Close()
	listeners := make([]net.Listener, 0, len(pairs))
	for _, pair := range pairs {
		listener, listenErr := net.Listen("tcp", pair.local)
		if listenErr != nil {
			for _, open := range listeners {
				_ = open.Close()
			}
			return fmt.Errorf("本地监听 %s 失败: %w", pair.local, listenErr)
		}
		listeners = append(listeners, listener)
	}
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	go func() {
		<-ctx.Done()
		for _, listener := range listeners {
			_ = listener.Close()
		}
		_ = client.Close()
	}()
	for _, pair := range pairs {
		ui.PrintSuccess("本地端口转发已启动: %s -> %s -> %s", pair.local, host.Alias, pair.remote)
	}
	fmt.Println("按 Ctrl+C 停止")
	return runLocalForwards(ctx, client, pairs, listeners)
}

type forwardPair struct {
	local  string
	remote string
}

// runLocalForward keeps the single-pair entry point used by tests.
func runLocalForward(ctx context.Context, client *ssh.Client, listener net.Listener, remoteTarget string) error {
	return runLocalForwards(ctx, client, []forwardPair{{remote: remoteTarget}}, []net.Listener{listener})
}

func runLocalForwards(ctx context.Context, client *ssh.Client, pairs []forwardPair, listeners []net.Listener) error {
	go func() {
		<-ctx.Done()
		for _, listener := range listeners {
			_ = listener.Close()
		}
		_ = client.Close()
	}()
	var wg sync.WaitGroup
	for index, listener := range listeners {
		remoteTarget := pairs[index].remote
		wg.Add(1)
		go func(listener net.Listener) {
			defer wg.Done()
			for {
				local, err := listener.Accept()
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					ui.PrintError("接受本地连接失败: %v", err)
					return
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer local.Close()
					remote, err := client.Dial("tcp", remoteTarget)
					if err != nil {
						ui.PrintError("转发连接失败: %v", err)
						return
					}
					defer remote.Close()
					done := make(chan struct{}, 2)
					go func() {
						_, _ = io.Copy(remote, local)
						done <- struct{}{}
					}()
					go func() {
						_, _ = io.Copy(local, remote)
						done <- struct{}{}
					}()
					<-done
				}()
			}
		}(listener)
	}
	wg.Wait()
	return nil
}
