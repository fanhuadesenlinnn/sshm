package ops

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/sshx"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestNativeExecutorUsesTransferAdapters(t *testing.T) {
	host := config.Host{Alias: "one"}
	executor := &NativeExecutor{
		PushFunc: func(_ context.Context, got config.Host, options TransferOptions) Result {
			if got.Alias != "one" || options.Dest != "/tmp/app" {
				t.Fatalf("host=%+v options=%+v", got, options)
			}
			return NewTransferResult(got, "sftp", options.Dest, nil, time.Second)
		},
	}
	result := executor.Push(context.Background(), host, TransferOptions{Dest: "/tmp/app"})
	if !result.OK || result.Method != "sftp" || result.Duration != time.Second {
		t.Fatalf("result = %+v", result)
	}
	result = executor.Pull(context.Background(), host, TransferOptions{})
	if result.OK || result.Stage != operation.StageTransfer {
		t.Fatalf("unavailable pull result = %+v", result)
	}
}

func TestNewTransferResultPreservesSpecificFailureStage(t *testing.T) {
	err := operation.Wrap(operation.StageNetwork, errors.New("offline"))
	result := NewTransferResult(config.Host{Alias: "one"}, "sftp", "", err, 0)
	if result.OK || result.Stage != operation.StageNetwork || !errors.Is(result.Err, err) {
		t.Fatalf("result = %+v", result)
	}
}

func TestNewTransferResultWithChange(t *testing.T) {
	result := NewTransferResultWithChange(config.Host{Alias: "one"}, "sftp", "/tmp/x", true, nil, time.Second)
	if !result.Changed || result.Destination != "/tmp/x" || result.Method != "sftp" || result.Duration != time.Second {
		t.Fatalf("result = %+v", result)
	}
}

func TestBecomeCommand(t *testing.T) {
	got := BecomeCommand("echo 'ok'", "deploy")
	want := `sudo -n -u 'deploy' -- sh -c 'echo '"'"'ok'"'"''`
	if got != want {
		t.Fatalf("BecomeCommand = %q, want %q", got, want)
	}
	if defaulted := BecomeCommand("uptime", ""); defaulted != `sudo -n -u 'root' -- sh -c 'uptime'` {
		t.Fatalf("默认 root 用户错误: %q", defaulted)
	}
}

type fakeFileInfo struct {
	name string
	size int64
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return nil }

type fakeStatResult struct {
	info    os.FileInfo
	err     error
	link    string
	linkErr error
}

type fakeSFTP struct {
	stat   map[string]fakeStatResult
	closed bool
}

func (f *fakeSFTP) Lstat(path string) (os.FileInfo, error) {
	result, ok := f.stat[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return result.info, result.err
}

func (f *fakeSFTP) ReadLink(path string) (string, error) {
	result := f.stat[path]
	return result.link, result.linkErr
}

func (f *fakeSFTP) Close() error {
	f.closed = true
	return nil
}

func statExecutor(t *testing.T, fake *fakeSFTP, dialErr error) (*NativeExecutor, *fakeSFTP) {
	t.Helper()
	return &NativeExecutor{
		DialSFTP: func(_ context.Context, host config.Host, _ time.Duration) (SFTPConn, error) {
			if dialErr != nil {
				return nil, dialErr
			}
			return fake, nil
		},
	}, fake
}

func TestNativeExecutorStatMissingFile(t *testing.T) {
	executor, fake := statExecutor(t, &fakeSFTP{stat: map[string]fakeStatResult{}}, nil)
	info, err := executor.Stat(context.Background(), config.Host{Alias: "one"}, "/nonexistent", 0)
	if err != nil {
		t.Fatalf("Stat 缺失文件应返回 nil error: %v", err)
	}
	if info.Exists {
		t.Fatalf("缺失文件 Exists 应为 false: %+v", info)
	}
	if !fake.closed {
		t.Fatal("SFTP 连接应当被关闭")
	}
}

func TestNativeExecutorStatRegularFile(t *testing.T) {
	entry := fakeStatResult{info: fakeFileInfo{name: "nginx.conf", size: 42, mode: 0o644}}
	executor, _ := statExecutor(t, &fakeSFTP{stat: map[string]fakeStatResult{"/etc/nginx/nginx.conf": entry}}, nil)
	info, err := executor.Stat(context.Background(), config.Host{Alias: "one"}, "/etc/nginx/nginx.conf", 0)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.Exists || info.IsDir || info.IsLink || info.Size != 42 || info.Mode.Perm() != 0o644 {
		t.Fatalf("Stat 普通文件 = %+v", info)
	}
}

func TestNativeExecutorStatDirectory(t *testing.T) {
	entry := fakeStatResult{info: fakeFileInfo{name: "data", mode: os.ModeDir | 0o755}}
	executor, _ := statExecutor(t, &fakeSFTP{stat: map[string]fakeStatResult{"/srv/data": entry}}, nil)
	info, err := executor.Stat(context.Background(), config.Host{Alias: "one"}, "/srv/data", 0)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir {
		t.Fatalf("目录 IsDir 应为 true: %+v", info)
	}
}

func TestNativeExecutorStatSymlink(t *testing.T) {
	entry := fakeStatResult{
		info: fakeFileInfo{name: "current", mode: os.ModeSymlink},
		link: "/opt/app/v1",
	}
	executor, _ := statExecutor(t, &fakeSFTP{stat: map[string]fakeStatResult{"/opt/app/current": entry}}, nil)
	info, err := executor.Stat(context.Background(), config.Host{Alias: "one"}, "/opt/app/current", 0)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsLink || info.Target != "/opt/app/v1" {
		t.Fatalf("软链 = %+v", info)
	}
}

func TestNativeExecutorStatReadLinkErrorKeepsResult(t *testing.T) {
	entry := fakeStatResult{
		info:    fakeFileInfo{name: "current", mode: os.ModeSymlink},
		linkErr: errors.New("readlink failed"),
	}
	executor, _ := statExecutor(t, &fakeSFTP{stat: map[string]fakeStatResult{"/opt/current": entry}}, nil)
	info, err := executor.Stat(context.Background(), config.Host{Alias: "one"}, "/opt/current", 0)
	if err != nil {
		t.Fatalf("ReadLink 失败不应让 Stat 失败: %v", err)
	}
	if !info.IsLink || info.Target != "" {
		t.Fatalf("软链 = %+v", info)
	}
}

func TestNativeExecutorStatLstatError(t *testing.T) {
	boom := errors.New("boom")
	entry := fakeStatResult{err: boom}
	executor, _ := statExecutor(t, &fakeSFTP{stat: map[string]fakeStatResult{"/x": entry}}, nil)
	if _, err := executor.Stat(context.Background(), config.Host{Alias: "one"}, "/x", 0); !errors.Is(err, boom) {
		t.Fatalf("Lstat 错误应透传: %v", err)
	}
}

func TestNativeExecutorStatDialError(t *testing.T) {
	boom := errors.New("dial failed")
	executor, _ := statExecutor(t, &fakeSFTP{}, boom)
	if _, err := executor.Stat(context.Background(), config.Host{Alias: "one"}, "/x", 0); !errors.Is(err, boom) {
		t.Fatalf("Dial 错误应透传: %v", err)
	}
}

func TestIsNoSuchFile(t *testing.T) {
	if !isNoSuchFile(os.ErrNotExist) {
		t.Fatal("os.ErrNotExist 应视为文件不存在")
	}
	if !isNoSuchFile(fmt.Errorf("wrapped: %w", os.ErrNotExist)) {
		t.Fatal("包装的 os.ErrNotExist 应视为文件不存在")
	}
	status := &sftp.StatusError{Code: uint32(sftp.ErrSSHFxNoSuchFile)}
	if !isNoSuchFile(status) {
		t.Fatal("sftp StatusError(NoSuchFile) 应视为文件不存在")
	}
	if isNoSuchFile(errors.New("permission denied")) {
		t.Fatal("其他错误不应视为文件不存在")
	}
}

type fakeSSHClient struct {
	dials    int
	sessions int
	closed   bool
}

func (f *fakeSSHClient) NewSession() (*ssh.Session, error) {
	f.sessions++
	return nil, errors.New("fake session")
}

func (f *fakeSSHClient) Dial(network, address string) (net.Conn, error) {
	f.dials++
	return nil, errors.New("fake dial")
}

func (f *fakeSSHClient) Close() error {
	f.closed = true
	return nil
}

func (f *fakeSSHClient) Wait() error { return nil }

func TestNativeExecutorReusesSessionPerHost(t *testing.T) {
	dialed := &fakeSSHClient{}
	executor := &NativeExecutor{
		DialFunc: func(_ context.Context, host config.Host, _ time.Duration) (sshx.Client, error) {
			return dialed, nil
		},
	}
	one := config.Host{Alias: "one"}
	two := config.Host{Alias: "two"}
	first, err := executor.getSession(context.Background(), one, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := executor.getSession(context.Background(), one, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("同一主机的会话应复用")
	}
	if _, err := executor.getSession(context.Background(), two, 0); err != nil {
		t.Fatal(err)
	}
	if len(executor.sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(executor.sessions))
	}
	executor.CloseSessions()
	if len(executor.sessions) != 0 || !dialed.closed {
		t.Fatalf("CloseSessions 应清空并关闭连接: sessions=%d closed=%t", len(executor.sessions), dialed.closed)
	}
}

func TestNativeExecutorExecUsesCachedSession(t *testing.T) {
	dialed := &fakeSSHClient{}
	executor := &NativeExecutor{
		DialFunc: func(_ context.Context, host config.Host, _ time.Duration) (sshx.Client, error) {
			return dialed, nil
		},
	}
	host := config.Host{Alias: "one"}
	for range 2 {
		result := executor.Exec(context.Background(), host, ExecOptions{Command: "uptime"})
		if result.Err == nil || !strings.Contains(result.Err.Error(), "fake session") {
			t.Fatalf("Exec 应透传 fake session 错误: %v", result.Err)
		}
	}
	if dialed.sessions != 2 {
		t.Fatalf("两次 Exec 应各建一个 session: %d", dialed.sessions)
	}
	if len(executor.sessions) != 1 {
		t.Fatalf("两次 Exec 应复用同一 SSH 连接: %d", len(executor.sessions))
	}
}

func TestNativeExecutorDialTCPUsesCachedSession(t *testing.T) {
	dialed := &fakeSSHClient{}
	executor := &NativeExecutor{
		DialFunc: func(_ context.Context, host config.Host, _ time.Duration) (sshx.Client, error) {
			return dialed, nil
		},
	}
	host := config.Host{Alias: "one"}
	_, err := executor.DialTCP(context.Background(), host, "127.0.0.1:80", 0)
	if err == nil || !strings.Contains(err.Error(), "fake dial") {
		t.Fatalf("DialTCP 应透传 fake dial 错误: %v", err)
	}
	if dialed.dials != 1 || len(executor.sessions) != 1 {
		t.Fatalf("DialTCP 应复用缓存会话: dials=%d sessions=%d", dialed.dials, len(executor.sessions))
	}
}

func TestReusableSessionRejectsNonSSHClient(t *testing.T) {
	executor := &NativeExecutor{
		DialFunc: func(_ context.Context, host config.Host, _ time.Duration) (sshx.Client, error) {
			return &fakeSSHClient{}, nil
		},
	}
	_, _, err := executor.ReusableSession(context.Background(), config.Host{Alias: "one"}, 0)
	if err == nil || !strings.Contains(err.Error(), "传输复用") {
		t.Fatalf("ReusableSession 应拒绝非 *ssh.Client: %v", err)
	}
}
