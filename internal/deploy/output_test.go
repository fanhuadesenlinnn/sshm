package deploy

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func TestWritePlanJSONIncludesTaskArgs(t *testing.T) {
	args := argsNode(map[string]any{"cmd": "systemctl restart app", "chdir": "/opt/app"})
	plan := &Plan{
		Name: "update", Config: "<test>", Strategy: StrategyLinear,
		Batch:          batchOptionsForTest(),
		Timeout:        mustDuration(t, "30s"),
		ConnectTimeout: mustDuration(t, "10s"),
		Hosts:          []config.Host{},
		Tasks: []Task{
			{Name: "restart", Module: "command", Args: args},
		},
	}
	var buffer bytes.Buffer
	if err := WritePlanJSON(&buffer, plan); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	tasks := decoded["tasks"].([]any)
	first := tasks[0].(map[string]any)
	moduleArgs, ok := first["args"].(map[string]any)
	if !ok {
		t.Fatalf("plan JSON 缺少 args: %s", buffer.String())
	}
	if moduleArgs["cmd"] != "systemctl restart app" || moduleArgs["chdir"] != "/opt/app" {
		t.Fatalf("args = %+v", moduleArgs)
	}
	if !strings.Contains(buffer.String(), "systemctl restart app") {
		t.Fatalf("plan JSON 应包含模块参数: %s", buffer.String())
	}
}

func batchOptionsForTest() batch.Options {
	return batch.Options{Parallel: 4}
}

func mustDuration(t *testing.T, value string) config.Duration {
	t.Helper()
	duration, err := config.ParseHumanDuration(value)
	if err != nil {
		t.Fatal(err)
	}
	return config.Duration{Duration: duration}
}
