package batch

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func TestRunnerPreservesOrderAndParallelLimit(t *testing.T) {
	hosts := testHosts(8)
	var active atomic.Int32
	var peak atomic.Int32
	result, err := (Runner{Options: Options{Parallel: 3}}).Run(context.Background(), hosts, func(_ context.Context, host config.Host) Result {
		current := active.Add(1)
		for current > peak.Load() && !peak.CompareAndSwap(peak.Load(), current) {
		}
		time.Sleep(10 * time.Millisecond)
		active.Add(-1)
		return Result{Status: StatusOK, Detail: host.Alias}
	})
	if err != nil {
		t.Fatal(err)
	}
	if peak.Load() > 3 {
		t.Fatalf("peak parallelism = %d", peak.Load())
	}
	for i, item := range result.Results {
		if item.Host.Alias != hosts[i].Alias || item.Detail != hosts[i].Alias {
			t.Fatalf("result %d lost stable order: %+v", i, item)
		}
	}
}

func TestRunnerSerialWaitsForPreviousBatch(t *testing.T) {
	hosts := testHosts(4)
	var mu sync.Mutex
	var started []string
	releaseFirst := make(chan struct{})
	resultCh := make(chan RunResult)
	go func() {
		result, _ := (Runner{Options: Options{Parallel: 4, Serial: 2}}).Run(context.Background(), hosts, func(_ context.Context, host config.Host) Result {
			mu.Lock()
			started = append(started, host.Alias)
			mu.Unlock()
			if host.Alias == "host-0" || host.Alias == "host-1" {
				<-releaseFirst
			}
			return Result{Status: StatusOK}
		})
		resultCh <- result
	}()
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	if len(started) != 2 {
		t.Fatalf("started before releasing first serial batch = %v", started)
	}
	mu.Unlock()
	close(releaseFirst)
	result := <-resultCh
	if result.Summary.OK != 4 {
		t.Fatalf("summary = %+v", result.Summary)
	}
}

func TestRunnerStopsSchedulingAtFailureThreshold(t *testing.T) {
	hosts := testHosts(6)
	result, err := (Runner{Options: Options{Parallel: 1, MaxFailPercent: 34}}).Run(context.Background(), hosts, func(_ context.Context, host config.Host) Result {
		if host.Alias == "host-0" || host.Alias == "host-1" || host.Alias == "host-2" {
			return Result{Status: StatusFailed, Err: fmt.Errorf("failed")}
		}
		return Result{Status: StatusOK}
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Failed != 3 || result.Summary.Skipped != 3 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	if ExitCode(result) != 1 {
		t.Fatalf("exit code = %d", ExitCode(result))
	}
}

func TestRunnerCancellationMarksUnstartedSkipped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	hosts := testHosts(4)
	result, err := (Runner{Options: Options{Parallel: 1}}).Run(ctx, hosts, func(ctx context.Context, _ config.Host) Result {
		cancel()
		<-ctx.Done()
		return Result{Status: StatusFailed, Err: ctx.Err()}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Cancelled || result.Summary.Skipped != 3 || ExitCode(result) != 130 {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunnerBeforeBatchRunsAfterPreviousBatchAndStopsSafely(t *testing.T) {
	hosts := testHosts(4)
	var completed atomic.Int32
	result, err := (Runner{
		Options: Options{Parallel: 2, Serial: 2},
		BeforeBatch: func(start, end, total int) error {
			if start == 2 && completed.Load() != 2 {
				t.Fatalf("second batch started before first completed: %d", completed.Load())
			}
			if start == 2 {
				return fmt.Errorf("confirmation refused")
			}
			return nil
		},
	}).Run(context.Background(), hosts, func(_ context.Context, _ config.Host) Result {
		completed.Add(1)
		return Result{Status: StatusOK}
	})
	if err == nil || result.Summary.OK != 2 || result.Summary.Skipped != 2 || result.StopReason != "confirmation refused" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func testHosts(count int) []config.Host {
	hosts := make([]config.Host, count)
	for i := range hosts {
		hosts[i] = config.Host{ID: config.NewID(), Alias: fmt.Sprintf("host-%d", i)}
	}
	return hosts
}
