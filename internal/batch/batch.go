package batch

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
)

type Status string

const (
	StatusOK          Status = "ok"
	StatusChanged     Status = "changed"
	StatusWouldChange Status = "would-change"
	StatusFailed      Status = "failed"
	StatusUnreachable Status = "unreachable"
	StatusSkipped     Status = "skipped"
)

type Options struct {
	Parallel       int
	Serial         int
	FailFast       bool
	MaxFail        int
	MaxFailPercent int
}

func (o Options) Validate() error {
	if o.Parallel < 1 || o.Parallel > 128 {
		return fmt.Errorf("parallel 必须在 1 到 128 之间")
	}
	if o.Serial < 0 {
		return fmt.Errorf("serial 必须大于 0")
	}
	if o.MaxFail < 0 {
		return fmt.Errorf("max-fail 必须大于 0")
	}
	if o.MaxFailPercent < 0 || o.MaxFailPercent > 100 {
		return fmt.Errorf("max-fail-percent 必须在 1 到 100 之间")
	}
	return nil
}

type Result struct {
	Host          config.Host
	Status        Status
	Err           error
	Detail        string
	SkippedReason string
	Expected      bool
	Value         any
	StartedAt     time.Time
	EndedAt       time.Time
}

type Summary struct {
	OK          int
	Changed     int
	WouldChange int
	Failed      int
	Unreachable int
	Skipped     int
}

type RunResult struct {
	Results    []Result
	Summary    Summary
	Cancelled  bool
	StopReason string
}

type Task func(context.Context, config.Host) Result

type Runner struct {
	Options     Options
	BeforeBatch func(start, end, total int) error
	Progress    func(done, total int, result Result)
}

func (r Runner) Run(ctx context.Context, hosts []config.Host, task Task) (RunResult, error) {
	if err := r.Options.Validate(); err != nil {
		return RunResult{}, err
	}
	result := RunResult{Results: make([]Result, len(hosts))}
	if len(hosts) == 0 {
		return result, nil
	}
	serial := r.Options.Serial
	if serial == 0 || serial > len(hosts) {
		serial = len(hosts)
	}
	done := 0
	failures := 0
	for start := 0; start < len(hosts); start += serial {
		end := min(start+serial, len(hosts))
		if result.StopReason != "" || ctx.Err() != nil {
			reason := result.StopReason
			if reason == "" {
				reason = "用户中断或上下文取消"
			}
			for i := start; i < len(hosts); i++ {
				result.Results[i] = skipped(hosts[i], reason)
			}
			break
		}
		if r.BeforeBatch != nil {
			if err := r.BeforeBatch(start, end, len(hosts)); err != nil {
				result.StopReason = err.Error()
				for i := start; i < len(hosts); i++ {
					result.Results[i] = skipped(hosts[i], result.StopReason)
				}
				result.Summary = summarize(result.Results)
				return result, err
			}
		}
		batchResults, batchFailures, stopReason := r.runBatch(ctx, hosts[start:end], task, len(hosts), failures, &done)
		copy(result.Results[start:end], batchResults)
		failures += batchFailures
		if stopReason != "" {
			result.StopReason = stopReason
		}
	}
	result.Cancelled = ctx.Err() != nil
	result.Summary = summarize(result.Results)
	return result, nil
}

type indexedResult struct {
	index  int
	result Result
}

func (r Runner) runBatch(
	ctx context.Context,
	hosts []config.Host,
	task Task,
	total int,
	previousFailures int,
	done *int,
) ([]Result, int, string) {
	results := make([]Result, len(hosts))
	parallel := min(r.Options.Parallel, len(hosts))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan int)
	completed := make(chan indexedResult, parallel)
	var workers sync.WaitGroup
	for range parallel {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				started := time.Now()
				taskResult := task(ctx, hosts[index])
				taskResult.Host = hosts[index]
				if taskResult.Status == "" {
					if taskResult.Err != nil {
						taskResult.Status = StatusFailed
					} else {
						taskResult.Status = StatusOK
					}
				}
				if taskResult.StartedAt.IsZero() {
					taskResult.StartedAt = started
				}
				if taskResult.EndedAt.IsZero() {
					taskResult.EndedAt = time.Now()
				}
				completed <- indexedResult{index: index, result: taskResult}
			}
		}()
	}

	next := 0
	running := 0
	schedule := func() bool {
		if next >= len(hosts) || ctx.Err() != nil {
			return false
		}
		jobs <- next
		next++
		running++
		return true
	}
	for running < parallel && schedule() {
	}

	failures := 0
	stopReason := ""
	for running > 0 {
		item := <-completed
		running--
		results[item.index] = item.result
		*done++
		if r.Progress != nil {
			r.Progress(*done, total, item.result)
		}
		if failed(item.result.Status) {
			failures++
			if stopReason == "" {
				stopReason = r.stopReason(total, previousFailures+failures)
			}
		}
		if stopReason == "" && ctx.Err() == nil {
			schedule()
		}
	}
	close(jobs)
	workers.Wait()

	if stopReason == "" && ctx.Err() != nil {
		stopReason = "用户中断或上下文取消"
	}
	for next < len(hosts) {
		results[next] = skipped(hosts[next], stopReason)
		next++
	}
	return results, failures, stopReason
}

func (r Runner) stopReason(total, failures int) string {
	switch {
	case r.Options.FailFast && failures >= 1:
		return "fail-fast 已触发"
	case r.Options.MaxFail > 0 && failures >= r.Options.MaxFail:
		return fmt.Sprintf("max-fail=%d 已触发", r.Options.MaxFail)
	case r.Options.MaxFailPercent > 0 && failures >= int(math.Ceil(float64(total*r.Options.MaxFailPercent)/100)):
		return fmt.Sprintf("max-fail-percent=%d 已触发", r.Options.MaxFailPercent)
	default:
		return ""
	}
}

func skipped(host config.Host, reason string) Result {
	now := time.Now()
	return Result{
		Host: host, Status: StatusSkipped, SkippedReason: reason,
		StartedAt: now, EndedAt: now,
	}
}

func failed(status Status) bool {
	return status == StatusFailed || status == StatusUnreachable
}

func summarize(results []Result) Summary {
	var summary Summary
	for _, result := range results {
		switch result.Status {
		case StatusOK:
			summary.OK++
		case StatusChanged:
			summary.Changed++
		case StatusWouldChange:
			summary.WouldChange++
		case StatusFailed:
			summary.Failed++
		case StatusUnreachable:
			summary.Unreachable++
		case StatusSkipped:
			summary.Skipped++
		}
	}
	return summary
}

func ExitCode(result RunResult) int {
	if result.Cancelled {
		return 130
	}
	if result.Summary.Failed > 0 {
		return 1
	}
	for _, item := range result.Results {
		if item.Status == StatusSkipped && !item.Expected {
			return 1
		}
	}
	if result.Summary.Unreachable > 0 {
		return 2
	}
	return 0
}
