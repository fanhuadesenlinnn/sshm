package ui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func TestFilterPickerHostsFiltersAndPrioritizesPinned(t *testing.T) {
	hosts := []config.Host{
		{Alias: "web-old", Tags: []string{"prod"}, LastUsedAt: "2026-01-01T00:00:00Z"},
		{Alias: "db", Tags: []string{"prod"}, Pinned: true},
		{Alias: "web-new", Tags: []string{"prod"}, LastUsedAt: "2026-06-01T00:00:00Z"},
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

func TestRenderHostPickerKeepsLargeSelectionVisible(t *testing.T) {
	hosts := make([]config.Host, 20)
	for i := range hosts {
		hosts[i] = config.Host{Alias: fmt.Sprintf("host-%02d", i), User: "root", Host: "example.com", Port: 22}
	}
	var output bytes.Buffer
	renderHostPickerTo(&output, hosts, "", 15, 100)
	text := output.String()
	if !strings.Contains(text, "> host-15") || !strings.Contains(text, "显示 5-16 / 20") {
		t.Fatalf("selected host is not visible: %q", text)
	}
}
