package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
)

func TestDeployDiffIsNotWrittenToLogs(t *testing.T) {
	t.Setenv("SSHM_HOME", t.TempDir())
	plan := Plan{Profile: "app", Diff: true}
	result := RunResult{Profile: "app", Results: []HostResult{{
		HostAlias: "one", Status: batch.StatusWouldChange,
		Steps: []StepResult{{Name: "push", Type: "push", Status: batch.StatusWouldChange, Output: "-secret\n+new-secret\n"}},
	}}}
	path, err := WriteLog(plan, &result, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"one.json", "summary.json"} {
		data, err := os.ReadFile(filepath.Join(path, file))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "secret") {
			t.Fatalf("%s leaked diff: %s", file, data)
		}
		var decoded any
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
	}
}
