package deploy

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
)

// PlayEvent is one machine-readable event emitted while a play runs.
type PlayEvent struct {
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
	r.Event(PlayEvent{
		Type: EventTaskDone, Play: plan.Name, Task: task.DisplayName(taskIndex),
		TaskIndex: taskIndex, Host: item.Host.Alias, Status: string(item.Status),
		Reason: reason, DurationMS: detail.DurationMS, Timestamp: time.Now(),
	})
}

// emitFree emits one task_host_done event per task of a completed free-strategy
// host, matching the linear strategy's event shape. It runs on the batch main
// goroutine, so the Event callback never races with parallel workers.
func (r Runner) emitFree(plan *Plan, item batch.Result) {
	if r.Event == nil {
		return
	}
	state, ok := item.Value.(*hostState)
	if !ok {
		return
	}
	for index := range plan.Tasks {
		var taskResult TaskResult
		if index < len(state.tasks) {
			taskResult = state.tasks[index]
		} else {
			taskResult = TaskResult{
				Name: plan.Tasks[index].DisplayName(index), Module: plan.Tasks[index].Module,
				Status: batch.StatusSkipped, Reason: "主机已失败",
			}
		}
		r.Event(PlayEvent{
			Type: EventTaskDone, Play: plan.Name, Task: plan.Tasks[index].DisplayName(index),
			TaskIndex: index, Host: item.Host.Alias, Status: string(taskResult.Status),
			Reason: taskResult.Reason, DurationMS: taskResult.DurationMS, Timestamp: time.Now(),
		})
	}
}

func emitPlayStart(r Runner, plan *Plan) {
	if r.Event == nil {
		return
	}
	r.Event(PlayEvent{Type: EventPlayStart, Play: plan.Name, Timestamp: time.Now()})
}

func emitPlayDone(r Runner, plan *Plan, result RunResult) {
	if r.Event == nil {
		return
	}
	r.Event(PlayEvent{
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
