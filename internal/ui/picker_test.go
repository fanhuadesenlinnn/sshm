package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
)

func TestFilterPickerHostsFiltersAndPrioritizesPinned(t *testing.T) {
	hosts := []config.Host{
		{Alias: "web-old", Group: "prod", LastUsedAt: "2026-01-01T00:00:00Z"},
		{Alias: "db", Group: "prod", Pinned: true},
		{Alias: "web-new", Group: "prod", LastUsedAt: "2026-06-01T00:00:00Z"},
	}
	got := filterPickerHosts(hosts, "prod")
	if len(got) != 3 || got[0].Alias != "db" || got[1].Alias != "web-new" {
		t.Fatalf("unexpected picker order: %+v", got)
	}
	got = filterPickerHosts(hosts, "web new")
	if len(got) != 1 || got[0].Alias != "web-new" {
		t.Fatalf("unexpected picker filter: %+v", got)
	}
}

func TestRenderHostPickerUsesRawModeSafeNewlines(t *testing.T) {
	var output bytes.Buffer
	renderHostPickerTo(&output, []config.Host{{
		Alias: "prod",
		User:  "root",
		Host:  "example.com",
		Port:  22,
	}}, "", 0, 100)

	withoutCRLF := strings.ReplaceAll(output.String(), "\r\n", "")
	if strings.Contains(withoutCRLF, "\n") {
		t.Fatalf("picker output contains bare newline in raw mode: %q", output.String())
	}
}
