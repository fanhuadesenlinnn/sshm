package command

import (
	"errors"
	"testing"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ops"
)

func TestCredentialStageMapsToUnreachableAndExitCodeTwo(t *testing.T) {
	err := operation.Wrap(operation.StageCredential, errors.New("missing credentials"))
	if got := batchStatus(ops.Result{Stage: operation.StageCredential, Err: err}); got != batch.StatusUnreachable {
		t.Fatalf("batch status = %q", got)
	}
	if got := ExitCodeForError(err); got != 2 {
		t.Fatalf("exit code = %d, want 2", got)
	}
}

func TestParseBatchCLIOptionsExcludes(t *testing.T) {
	options, positionals, err := parseBatchCLIOptions([]string{
		"prod", "--exclude", "web02,db01", "--exclude-tag", "legacy", "--serial", "2", "--", "uptime",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Exclude) != 1 || options.Exclude[0] != "web02,db01" ||
		len(options.ExcludeTags) != 1 || options.ExcludeTags[0] != "legacy" {
		t.Fatalf("options = %+v", options)
	}
	if len(positionals) != 2 || positionals[0] != "prod" || positionals[1] != "uptime" {
		t.Fatalf("positionals = %v", positionals)
	}
}
