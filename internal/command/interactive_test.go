package command

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
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
		{`cmd ""`, []string{"cmd", ""}},
		{`cmd escaped\ space`, []string{"cmd", "escaped space"}},
		{`cmd "C:\Program Files\Editor\editor.exe"`, []string{"cmd", `C:\Program Files\Editor\editor.exe`}},
		{`cmd "\\server\share path"`, []string{"cmd", `\\server\share path`}},
		{"  leading space", []string{"leading", "space"}},
		{"trailing space  ", []string{"trailing", "space"}},
		{"multiple   spaces", []string{"multiple", "spaces"}},
	}

	for _, tt := range tests {
		got, err := parseArgs(tt.input)
		if err != nil {
			t.Errorf("parseArgs(%q) error = %v", tt.input, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseArgs(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseArgsPreservesLongQuotedRemoteCommand(t *testing.T) {
	command := `for h in /sys/class/fc_host/host*; do printf "%s WWPN=%s WWNN=%s State=%s Speed=%s\n" "$(basename "$h")" "$(cat "$h/port_name")" "$(cat "$h/node_name")" "$(cat "$h/port_state")" "$(cat "$h/speed")"; done`
	parts, err := parseInteractiveInput("xt temk '" + command + "'")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"xt", "temk", "--", command}
	if !reflect.DeepEqual(parts, want) {
		t.Fatalf("parts = %#v, want %#v", parts, want)
	}
}

func TestParseArgsRejectsUnclosedQuotes(t *testing.T) {
	for _, input := range []string{`cmd "unfinished`, `cmd 'unfinished`} {
		if _, err := parseArgs(input); err == nil {
			t.Fatalf("parseArgs(%q) should reject unclosed quotes", input)
		}
	}
}

func TestInteractiveAliasRouting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	store := config.NewStoreWithPath(path)
	if err := store.Repository().Replace(config.DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: path}
	for _, name := range []string{"f", "find-con", "pick"} {
		exit, err := app.dispatchInteractive([]string{name})
		if exit {
			t.Fatalf("%s 不应退出工作台", name)
		}
		if err == nil || !strings.Contains(err.Error(), "暂无主机") {
			t.Fatalf("%s 应路由到 find-con（打开选择器）: err=%v", name, err)
		}
	}
	for _, name := range []string{"p", "ping"} {
		exit, err := app.dispatchInteractive([]string{name})
		if exit || err != nil {
			t.Fatalf("%s 应路由到 ping: exit=%v err=%v", name, exit, err)
		}
	}
	if exit, _ := app.dispatchInteractive([]string{"q"}); !exit {
		t.Fatal("q 应退出工作台")
	}
}

func TestCommandNamesForCompletionIncludesRenamedAliases(t *testing.T) {
	names := commandNamesForCompletion()
	for _, want := range []string{"find-con", "f", "pick", "ping", "p"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("补全候选缺少 %q: %v", want, names)
		}
	}
}

func TestParseInteractiveExecPreservesRemoteCommand(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{
			input: `x web01 awk '{print $1}' /tmp/data`,
			want:  []string{"x", "web01", "--", `awk '{print $1}' /tmp/data`},
		},
		{
			input: `x web01 --quiet pwd`,
			want:  []string{"x", "--quiet", "web01", "--", "pwd"},
		},
		{
			input: `x --quiet web01 pwd --quiet`,
			want:  []string{"x", "--quiet", "web01", "--", "pwd --quiet"},
		},
		{
			input: `x web01 -- echo "--quiet"`,
			want:  []string{"x", "web01", "--", `echo "--quiet"`},
		},
		{
			input: `x web01 printf '%s\n' "a b"`,
			want:  []string{"x", "web01", "--", `printf '%s\n' "a b"`},
		},
		{
			input: `x web01 -- --quiet pwd`,
			want:  []string{"x", "web01", "--", "--quiet pwd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseInteractiveInput(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parts = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseInteractiveExecTagAcceptsOptionsAroundTarget(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{
			input: `xt prod --parallel 4 --yes uptime`,
			want:  []string{"xt", "--parallel", "4", "--yes", "prod", "--", "uptime"},
		},
		{
			input: `xt --parallel 4 prod -- systemctl restart app`,
			want:  []string{"xt", "--parallel", "4", "prod", "--", "systemctl restart app"},
		},
	}
	for _, tt := range tests {
		got, err := parseInteractiveInput(tt.input)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("parseInteractiveInput(%q) = %#v, want %#v", tt.input, got, tt.want)
		}
	}
}

func TestParseInteractiveExecExplainsUnknownOptionBoundary(t *testing.T) {
	_, err := parseInteractiveInput(`x web01 --quie pwd`)
	if err == nil || !strings.Contains(err.Error(), "前面加 --") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInteractiveExecParsingReachesCommandHandlersUnchanged(t *testing.T) {
	parts, err := parseInteractiveInput(`x web01 --quiet -- awk '{print $1}' /tmp/data`)
	if err != nil {
		t.Fatal(err)
	}
	yes, quiet, noLog, host, command, err := parseExecArgs(parts[1:])
	if err != nil {
		t.Fatal(err)
	}
	if yes || !quiet || noLog || host != "web01" || command != `awk '{print $1}' /tmp/data` {
		t.Fatalf("unexpected exec parse: yes=%t quiet=%t noLog=%t host=%q command=%q", yes, quiet, noLog, host, command)
	}

	parts, err = parseInteractiveInput(`xt prod --parallel 4 --yes -- printf '%s\n' "a b"`)
	if err != nil {
		t.Fatal(err)
	}
	options, positionals, err := parseBatchCLIOptions(parts[1:])
	if err != nil {
		t.Fatal(err)
	}
	if options.Parallel != 4 || !options.Yes {
		t.Fatalf("unexpected batch options: %+v", options)
	}
	wantPositionals := []string{"prod", `printf '%s\n' "a b"`}
	if !reflect.DeepEqual(positionals, wantPositionals) {
		t.Fatalf("positionals = %#v, want %#v", positionals, wantPositionals)
	}
}

func TestInteractiveExecDelimiterRequiresCommand(t *testing.T) {
	for _, input := range []string{`x web01 --`, `xt prod --`} {
		if _, err := parseInteractiveInput(input); err == nil || !strings.Contains(err.Error(), "缺少远程命令") {
			t.Fatalf("parseInteractiveInput(%q) error = %v", input, err)
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
	if !shouldTryTemporaryPassword(operation.Wrap(operation.StageCredential, errors.New("missing credentials"))) {
		t.Fatal("missing credentials should allow temporary password retry")
	}
	for _, stage := range []operation.FailureStage{
		operation.StageResolve,
		operation.StageNetwork,
		operation.StageJump,
		operation.StageTrust,
		operation.StageVault,
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
