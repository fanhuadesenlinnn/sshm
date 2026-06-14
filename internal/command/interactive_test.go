package command

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v4/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/operation"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"hello", []string{"hello"}},
		{"hello world", []string{"hello", "world"}},
		{"cmd arg1 arg2", []string{"cmd", "arg1", "arg2"}},
		{`cmd "arg with spaces"`, []string{"cmd", "arg with spaces"}},
		{`cmd 'single quoted'`, []string{"cmd", "single quoted"}},
		{"  leading space", []string{"leading", "space"}},
		{"trailing space  ", []string{"trailing", "space"}},
		{"multiple   spaces", []string{"multiple", "spaces"}},
	}

	for _, tt := range tests {
		got := parseArgs(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseArgs(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestMatchHost(t *testing.T) {
	h := config.Host{
		Alias: "webserver",
		Host:  "10.0.0.1",
		User:  "admin",
		Note:  "production web server",
		Tags:  []string{"web", "nginx"},
	}

	tests := []struct {
		keyword string
		want    bool
	}{
		{"web", true},        // matches alias
		{"server", true},     // matches alias
		{"10.0", true},       // matches host
		{"admin", true},      // matches user
		{"production", true}, // matches note
		{"nginx", true},      // matches tag
		{"nonexistent", false},
		{"database", false},
		{"", true}, // empty keyword matches everything (contains check)
	}

	for _, tt := range tests {
		got := matchHost(h, tt.keyword)
		if got != tt.want {
			t.Errorf("matchHost(host, %q) = %v, want %v", tt.keyword, got, tt.want)
		}
	}
}

func TestTemporaryPasswordRetryOnlyFollowsAuthenticationFailure(t *testing.T) {
	if !shouldTryTemporaryPassword(operation.Wrap(operation.StageAuth, errors.New("bad credentials"))) {
		t.Fatal("authentication failure should allow temporary password retry")
	}
	for _, stage := range []operation.FailureStage{
		operation.StageResolve,
		operation.StageNetwork,
		operation.StageJump,
		operation.StageTrust,
		operation.StageSession,
	} {
		if shouldTryTemporaryPassword(operation.Wrap(stage, errors.New("failed"))) {
			t.Fatalf("%s failure should not prompt for a temporary password", stage)
		}
	}
}

func TestParseSSHConfigMultipleAliases(t *testing.T) {
	hosts := parseSSHConfig(`
Host one two *.example
    HostName example.com
    User deploy
    Port 2222
    IdentityFile "~/.ssh/id key"
`)
	if len(hosts) != 2 {
		t.Fatalf("len(hosts) = %d, want 2", len(hosts))
	}
	for _, h := range hosts {
		if h.Host != "example.com" || h.User != "deploy" || h.Port != 2222 {
			t.Fatalf("unexpected host: %+v", h)
		}
	}
}
