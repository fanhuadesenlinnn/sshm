package ops

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
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
