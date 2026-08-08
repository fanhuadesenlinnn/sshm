package command

import (
	"errors"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
)

func TestCredentialFailureSuggestsConcreteNextCommand(t *testing.T) {
	err := operation.Wrap(operation.StageCredential, errors.New("missing credentials"))
	result := newOperationResult(
		config.Host{Alias: "web01", Auth: "auto"},
		"", err, operation.StageExecute, `sshmd exec --yes 'web01' 'uptime'`, time.Second,
	)
	if result.RetryCommand != "sshmd passwd web01" {
		t.Fatalf("retry command = %q", result.RetryCommand)
	}

	result = newOperationResult(
		config.Host{Alias: "web01", Auth: "key", Identity: config.ManagedIdentity("personal")},
		"", err, operation.StageExecute, `sshmd exec --yes 'web01' 'uptime'`, time.Second,
	)
	if result.RetryCommand != "sshmd key setup personal web01 --yes" {
		t.Fatalf("managed key retry command = %q", result.RetryCommand)
	}
}

func TestNonCredentialFailureKeepsOriginalRetryCommand(t *testing.T) {
	err := operation.Wrap(operation.StageExecute, errors.New("remote command failed"))
	result := newOperationResult(
		config.Host{Alias: "web01", Auth: "auto"},
		"", err, operation.StageExecute, `sshmd exec --yes 'web01' 'uptime'`, time.Second,
	)
	if result.RetryCommand != `sshmd exec --yes 'web01' 'uptime'` {
		t.Fatalf("retry command = %q", result.RetryCommand)
	}
}
