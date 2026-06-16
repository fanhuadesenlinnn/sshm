package command

import (
	"errors"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
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
