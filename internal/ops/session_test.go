package ops

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/sshx"
	"golang.org/x/crypto/ssh"
)

type concurrentFakeClient struct {
	closed     chan struct{}
	closeOnce  sync.Once
	closeCount atomic.Int32
	dialStart  chan struct{}
}

func newConcurrentFakeClient() *concurrentFakeClient {
	return &concurrentFakeClient{closed: make(chan struct{})}
}

func (f *concurrentFakeClient) NewSession() (*ssh.Session, error) {
	return nil, errors.New("fake session")
}

func (f *concurrentFakeClient) Dial(string, string) (net.Conn, error) {
	if f.dialStart != nil {
		select {
		case f.dialStart <- struct{}{}:
		default:
		}
	}
	<-f.closed
	return nil, errors.New("fake dial closed")
}

func (f *concurrentFakeClient) Close() error {
	f.closeCount.Add(1)
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

func (f *concurrentFakeClient) Wait() error { return nil }

func TestSessionManagerCoalescesSameHostDial(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var dials atomic.Int32
	client := newConcurrentFakeClient()
	manager := newSessionManager(func(context.Context, config.Host, time.Duration) (sshx.Client, error) {
		dials.Add(1)
		started <- struct{}{}
		<-release
		return client, nil
	}, nil)
	host := config.Host{Alias: "same"}
	type result struct {
		session *hostSession
		err     error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			session, err := manager.acquire(context.Background(), host, time.Second)
			results <- result{session: session, err: err}
		}()
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("拨号没有开始")
	}
	close(release)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("acquire errors: %v, %v", first.err, second.err)
	}
	if first.session != second.session || dials.Load() != 1 {
		t.Fatalf("同一主机应合并拨号: same=%t dials=%d", first.session == second.session, dials.Load())
	}
	manager.closeAll()
}

func TestSessionManagerCanceledWaiterDoesNotPoisonSharedDial(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var dials atomic.Int32
	manager := newSessionManager(func(ctx context.Context, _ config.Host, _ time.Duration) (sshx.Client, error) {
		dials.Add(1)
		close(started)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return newConcurrentFakeClient(), nil
		}
	}, nil)
	host := config.Host{Alias: "shared"}
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leader := make(chan error, 1)
	go func() {
		_, err := manager.acquire(leaderCtx, host, time.Second)
		leader <- err
	}()
	<-started
	waiter := make(chan error, 1)
	go func() {
		_, err := manager.acquire(context.Background(), host, time.Second)
		waiter <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		manager.mu.Lock()
		call := manager.inflight[host.Alias]
		joined := call != nil && call.waiters == 2
		manager.mu.Unlock()
		if joined {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("第二个调用未加入共享拨号")
		}
		time.Sleep(time.Millisecond)
	}
	cancelLeader()
	if err := <-leader; !errors.Is(err, context.Canceled) {
		t.Fatalf("首个等待者取消错误 = %v", err)
	}
	close(release)
	if err := <-waiter; err != nil {
		t.Fatalf("其他等待者不应被首个上下文取消: %v", err)
	}
	if dials.Load() != 1 {
		t.Fatalf("共享拨号次数 = %d", dials.Load())
	}
	manager.closeAll()
}

func TestSessionManagerDialsDifferentHostsInParallel(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	manager := newSessionManager(func(_ context.Context, host config.Host, _ time.Duration) (sshx.Client, error) {
		started <- host.Alias
		<-release
		return newConcurrentFakeClient(), nil
	}, nil)
	results := make(chan error, 2)
	for _, alias := range []string{"one", "two"} {
		go func(alias string) {
			_, err := manager.acquire(context.Background(), config.Host{Alias: alias}, time.Second)
			results <- err
		}(alias)
	}

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case alias := <-started:
			seen[alias] = true
		case <-time.After(time.Second):
			close(release)
			t.Fatalf("不同主机拨号被串行化: started=%v", seen)
		}
	}
	close(release)
	if err := <-results; err != nil {
		t.Fatal(err)
	}
	if err := <-results; err != nil {
		t.Fatal(err)
	}
	manager.closeAll()
}

func TestSessionManagerDoesNotCacheFailedDial(t *testing.T) {
	var dials atomic.Int32
	manager := newSessionManager(func(context.Context, config.Host, time.Duration) (sshx.Client, error) {
		if dials.Add(1) == 1 {
			return nil, errors.New("dial failed")
		}
		return newConcurrentFakeClient(), nil
	}, nil)
	host := config.Host{Alias: "retry"}
	if _, err := manager.acquire(context.Background(), host, time.Second); err == nil {
		t.Fatal("第一次拨号应失败")
	}
	if len(manager.sessions) != 0 || len(manager.inflight) != 0 {
		t.Fatalf("失败拨号不应留下状态: sessions=%d inflight=%d", len(manager.sessions), len(manager.inflight))
	}
	if _, err := manager.acquire(context.Background(), host, time.Second); err != nil {
		t.Fatalf("后续拨号应可恢复: %v", err)
	}
	if dials.Load() != 2 {
		t.Fatalf("dials=%d, want 2", dials.Load())
	}
	manager.closeAll()
}

func TestSessionManagerRejectsNilDialClient(t *testing.T) {
	manager := newSessionManager(func(context.Context, config.Host, time.Duration) (sshx.Client, error) {
		return nil, nil
	}, nil)
	if _, err := manager.acquire(context.Background(), config.Host{Alias: "nil"}, time.Second); err == nil {
		t.Fatal("空 SSH 客户端不应作为成功会话缓存")
	}
	if len(manager.sessions) != 0 || len(manager.inflight) != 0 {
		t.Fatal("空 SSH 客户端不应留下缓存状态")
	}
}

func TestSessionManagerRejectsNilSFTPConnection(t *testing.T) {
	client := newConcurrentFakeClient()
	manager := newSessionManager(
		func(context.Context, config.Host, time.Duration) (sshx.Client, error) { return client, nil },
		func(sshx.Client) (SFTPConn, error) { return nil, nil },
	)
	host := config.Host{Alias: "nil-sftp"}
	session, err := manager.acquire(context.Background(), host, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.sftpFor(context.Background(), host.Alias, session); err == nil {
		t.Fatal("空 SFTP 客户端不应作为成功连接缓存")
	}
	if len(manager.sessions) != 0 || client.closeCount.Load() == 0 {
		t.Fatal("SFTP 打开失败应关闭并淘汰 SSH 会话")
	}
}

func TestSessionManagerCloseAllDiscardsInflightDial(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	client := newConcurrentFakeClient()
	manager := newSessionManager(func(context.Context, config.Host, time.Duration) (sshx.Client, error) {
		close(started)
		<-release // Deliberately ignore cancellation to exercise late completion.
		return client, nil
	}, nil)
	result := make(chan error, 1)
	go func() {
		_, err := manager.acquire(context.Background(), config.Host{Alias: "one"}, time.Second)
		result <- err
	}()
	<-started
	manager.closeAll()
	close(release)
	if err := <-result; !errors.Is(err, errSessionInvalidated) {
		t.Fatalf("迟到拨号应被丢弃: %v", err)
	}
	if client.closeCount.Load() == 0 || len(manager.sessions) != 0 || len(manager.inflight) != 0 {
		t.Fatalf("迟到连接泄漏: closes=%d sessions=%d inflight=%d", client.closeCount.Load(), len(manager.sessions), len(manager.inflight))
	}
}

type blockingSFTP struct {
	started   chan struct{}
	closed    chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
}

func newBlockingSFTP() *blockingSFTP {
	return &blockingSFTP{started: make(chan struct{}), closed: make(chan struct{})}
}

func (f *blockingSFTP) Lstat(string) (os.FileInfo, error) {
	f.startOnce.Do(func() { close(f.started) })
	<-f.closed
	return nil, errors.New("SFTP closed")
}

func (f *blockingSFTP) ReadLink(string) (string, error) {
	return "", errors.New("SFTP closed")
}

func (f *blockingSFTP) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

func TestNativeExecutorStatCancellationClosesAndEvictsSession(t *testing.T) {
	client := newConcurrentFakeClient()
	sftpConn := newBlockingSFTP()
	executor := &NativeExecutor{
		DialFunc: func(context.Context, config.Host, time.Duration) (sshx.Client, error) {
			return client, nil
		},
		SFTPOpen: func(sshx.Client) (SFTPConn, error) { return sftpConn, nil },
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := executor.Stat(ctx, config.Host{Alias: "one"}, "/blocked", time.Second)
	if operation.StageOf(err, operation.StageUnknown) != operation.StageTimeout {
		t.Fatalf("Stat 取消阶段错误: %v", err)
	}
	if client.closeCount.Load() == 0 {
		t.Fatal("Stat 取消应关闭 SSH 会话")
	}
	if len(executor.sessionManager().sessions) != 0 {
		t.Fatal("Stat 取消应淘汰缓存会话")
	}
}

func TestSessionManagerCanceledWaiterDoesNotInterruptActiveOperation(t *testing.T) {
	client := newConcurrentFakeClient()
	manager := newSessionManager(func(context.Context, config.Host, time.Duration) (sshx.Client, error) {
		return client, nil
	}, nil)
	host := config.Host{Alias: "one"}
	active, releaseActive, err := manager.beginOperation(context.Background(), host, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := manager.beginOperation(ctx, host, time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting operation error = %v", err)
	}
	if client.closeCount.Load() != 0 || manager.sessions[host.Alias] != active {
		t.Fatal("canceling a waiter must not close the active shared session")
	}
	releaseActive()
	_, releaseNext, err := manager.beginOperation(context.Background(), host, time.Second)
	if err != nil {
		t.Fatalf("session should remain usable: %v", err)
	}
	releaseNext()
	manager.closeAll()
}

func TestSessionManagerLateInvalidationCannotCloseReplacementGeneration(t *testing.T) {
	first := newConcurrentFakeClient()
	second := newConcurrentFakeClient()
	clients := []sshx.Client{first, second}
	manager := newSessionManager(func(context.Context, config.Host, time.Duration) (sshx.Client, error) {
		client := clients[0]
		clients = clients[1:]
		return client, nil
	}, nil)
	host := config.Host{Alias: "one"}
	oldSession, releaseOld, err := manager.beginOperation(context.Background(), host, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	manager.invalidate(host.Alias, oldSession)
	releaseOld()
	newSession, releaseNew, err := manager.beginOperation(context.Background(), host, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	manager.invalidate(host.Alias, oldSession)
	if manager.sessions[host.Alias] != newSession || second.closeCount.Load() != 0 {
		t.Fatal("late invalidation from an old operation closed the replacement session")
	}
	releaseNew()
	manager.closeAll()
}

func TestNativeExecutorCanceledAcquireIsClassifiedAsTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := (&NativeExecutor{}).Exec(ctx, config.Host{Alias: "one"}, ExecOptions{Command: "true"})
	if result.Stage != operation.StageTimeout || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("canceled Exec result = %+v", result)
	}
}

func TestNativeExecutorDialTCPCancellationClosesAndEvictsSession(t *testing.T) {
	client := newConcurrentFakeClient()
	client.dialStart = make(chan struct{}, 1)
	executor := &NativeExecutor{
		DialFunc: func(context.Context, config.Host, time.Duration) (sshx.Client, error) {
			return client, nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := executor.DialTCP(ctx, config.Host{Alias: "one"}, "127.0.0.1:80", time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("DialTCP 取消错误 = %v", err)
	}
	if len(executor.sessionManager().sessions) != 0 || client.closeCount.Load() == 0 {
		t.Fatal("DialTCP 取消应关闭并淘汰缓存会话")
	}
}
