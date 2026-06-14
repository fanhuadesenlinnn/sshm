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
	if len(args) != 3 {
		return fmt.Errorf("用法: sshm forward <别名|ID> <本地监听地址> <远程目标>\n示例: sshm forward prod 127.0.0.1:8080 127.0.0.1:80")
	}
	if _, _, err := net.SplitHostPort(args[1]); err != nil {
		return fmt.Errorf("无效本地监听地址 %q: %w", args[1], err)
	}
	if _, _, err := net.SplitHostPort(args[2]); err != nil {
		return fmt.Errorf("无效远程目标 %q: %w", args[2], err)
	}
	host, _, _, err := app.Store.FindHost(args[0])
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
	listener, err := net.Listen("tcp", args[1])
	if err != nil {
		return fmt.Errorf("本地监听失败: %w", err)
	}
	defer listener.Close()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
		_ = client.Close()
	}()
	ui.PrintSuccess("本地端口转发已启动: %s -> %s -> %s", args[1], host.Alias, args[2])
	fmt.Println("按 Ctrl+C 停止")
	return runLocalForward(ctx, client, listener, args[2])
}

func runLocalForward(ctx context.Context, client *ssh.Client, listener net.Listener, remoteTarget string) error {
	go func() {
		<-ctx.Done()
		_ = listener.Close()
		_ = client.Close()
	}()
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		local, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("接受本地连接失败: %w", err)
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
}
