package deployv3

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
)

// V3Event is one machine-readable event emitted while a play runs.
type V3Event struct {
	Type       string        `json:"type"`
	Play       string        `json:"play,omitempty"`
	Task       string        `json:"task,omitempty"`
	TaskIndex  int           `json:"task_index,omitempty"`
	Host       string        `json:"host,omitempty"`
	Status     string        `json:"status,omitempty"`
	Reason     string        `json:"reason,omitempty"`
	DurationMS int64         `json:"duration_ms,omitempty"`
	Summary    batch.Summary `json:"summary,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
}

const (
	EventPlayStart = "play_start"
	EventPlayDone  = "play_done"
	EventTaskDone  = "task_host_done"
)

func (r Runner) emit(plan *Plan, taskIndex int, task Task, item batch.Result) {
	if r.Event == nil {
		return
	}
	detail, _ := item.Value.(TaskResult)
	reason := detail.Reason
	if item.SkippedReason != "" {
		reason = item.SkippedReason
	}
	r.Event(V3Event{
		Type: EventTaskDone, Play: plan.Name, Task: task.DisplayName(taskIndex),
		TaskIndex: taskIndex, Host: item.Host.Alias, Status: string(item.Status),
		Reason: reason, DurationMS: detail.DurationMS, Timestamp: time.Now(),
	})
}

func (r Runner) emitFree(plan *Plan, done, total int, item batch.Result) {
	if r.Event == nil {
		return
	}
	state, _ := item.Value.(*hostState)
	r.Event(V3Event{
		Type: EventTaskDone, Play: plan.Name, Host: item.Host.Alias,
		Status: string(item.Status), Reason: item.Detail, Timestamp: time.Now(),
	})
	_ = state
	_ = done
	_ = total
}

func emitPlayStart(r Runner, plan *Plan) {
	if r.Event == nil {
		return
	}
	r.Event(V3Event{Type: EventPlayStart, Play: plan.Name, Timestamp: time.Now()})
}

func emitPlayDone(r Runner, plan *Plan, result RunResult) {
	if r.Event == nil {
		return
	}
	r.Event(V3Event{
		Type: EventPlayDone, Play: plan.Name, Status: "done",
		Summary: result.Summary, Timestamp: time.Now(),
	})
}

// WriteNDJSON writes one JSON object per line.
func WriteNDJSON(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(writer, string(data))
	return err
}
