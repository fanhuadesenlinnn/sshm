package sshx

import (
	"slices"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
)

func TestKeyExecAndPingArgsUseConfiguredIdentity(t *testing.T) {
	h := config.Host{User: "root", Host: "example.com", Port: 2222, Identity: "/tmp/id key"}
	for name, args := range map[string][]string{
		"exec": buildExecArgs(h, "uptime", true),
		"ping": buildPingArgs(h, true),
	} {
		if !slices.Contains(args, "-i") || !slices.Contains(args, "/tmp/id key") {
			t.Fatalf("%s args do not use identity: %v", name, args)
		}
		if !slices.Contains(args, "IdentitiesOnly=yes") {
			t.Fatalf("%s args do not set IdentitiesOnly: %v", name, args)
		}
	}
}

func TestSystemArgsDoNotOverrideAuthentication(t *testing.T) {
	h := config.Host{User: "root", Host: "example.com", Port: 22, Identity: "/tmp/id"}
	for _, args := range [][]string{
		buildConnectArgs(h, nil, false),
		buildExecArgs(h, "true", false),
		buildPingArgs(h, false),
	} {
		if slices.Contains(args, "-i") || slices.Contains(args, "IdentitiesOnly=yes") {
			t.Fatalf("system args override authentication: %v", args)
		}
	}
}

func TestConnectionCommandIncludesConfiguredIdentity(t *testing.T) {
	h := config.Host{
		User:     "root",
		Host:     "example.com",
		Port:     2222,
		Identity: "/tmp/id key",
		Auth:     "auto",
	}
	got := ConnectionCommand(h)
	want := "ssh -p 2222 -i '/tmp/id key' -o IdentitiesOnly=yes root@example.com"
	if got != want {
		t.Fatalf("ConnectionCommand() = %q, want %q", got, want)
	}
}
