package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
)

func TestRenderHostsTableDefaultUsesContentSizedOptionalColumns(t *testing.T) {
	output := renderHostsTableForTest(t, sampleHosts(), HostTableOptions{Width: 200})

	for _, want := range []string{
		"ID", "主机", "连接目标", "标签", "备注",
		"root@45.62.111.213", "deploy@10.0.0.12:2222",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output does not contain %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"认证", "信任", "跳板机"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("default output unexpectedly contains %q:\n%s", unwanted, output)
		}
	}
	if got := widestLine(output); got > 90 {
		t.Fatalf("default output expanded to %d columns:\n%s", got, output)
	}
}

func TestRenderHostsTableDefaultOmitsEmptyOptionalColumns(t *testing.T) {
	output := renderHostsTableForTest(t, sampleHosts()[:1], HostTableOptions{Width: 200})

	for _, unwanted := range []string{"标签", "备注", "认证"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("default output unexpectedly contains %q:\n%s", unwanted, output)
		}
	}
}

func TestRenderHostsTableCompactIsMinimal(t *testing.T) {
	output := renderHostsTableForTest(t, sampleHosts(), HostTableOptions{Compact: true, Width: 200})

	for _, want := range []string{"1", "prod-web", "deploy@10.0.0.12:2222"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output does not contain %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"ID", "连接目标", "认证", "标签", "备注", "prod web", "主站应用服务"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("compact output unexpectedly contains %q:\n%s", unwanted, output)
		}
	}
}

func TestRenderHostsTableWideShowsDiagnosticColumns(t *testing.T) {
	output := renderHostsTableForTest(t, sampleHosts(), HostTableOptions{Wide: true, Width: 240})

	for _, want := range []string{
		"认证", "信任", "跳板机", "标签", "备注",
		"key", "accept-new", "45.62.111.213", "prod web", "主站应用服务",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("wide output does not contain %q:\n%s", want, output)
		}
	}
}

func TestRenderHostsTableNarrowDropsOptionalColumns(t *testing.T) {
	const width = 42
	output := renderHostsTableForTest(t, sampleHosts(), HostTableOptions{Width: width})

	for _, unwanted := range []string{"标签", "备注"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("narrow output unexpectedly contains %q:\n%s", unwanted, output)
		}
	}
	if got := widestLine(output); got > width {
		t.Fatalf("narrow output is %d columns, want at most %d:\n%s", got, width, output)
	}
}

func TestDisplayConnectionTarget(t *testing.T) {
	tests := []struct {
		name string
		host config.Host
		want string
	}{
		{name: "default port", host: config.Host{User: "root", Host: "example.com", Port: 22}, want: "root@example.com"},
		{name: "custom port", host: config.Host{User: "root", Host: "example.com", Port: 2222}, want: "root@example.com:2222"},
		{name: "IPv6", host: config.Host{User: "root", Host: "2001:db8::1", Port: 2222}, want: "root@[2001:db8::1]:2222"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayConnectionTarget(tt.host); got != tt.want {
				t.Fatalf("displayConnectionTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func renderHostsTableForTest(t *testing.T, hosts []config.Host, options HostTableOptions) string {
	t.Helper()
	var output bytes.Buffer
	renderHostsTableTo(&output, hosts, options)
	return output.String()
}

func sampleHosts() []config.Host {
	return []config.Host{
		{
			Alias:                 "45.62.111.213",
			User:                  "root",
			Host:                  "45.62.111.213",
			Port:                  22,
			Auth:                  "auto",
			ResolvedHostKeyPolicy: config.HostKeyPolicyStrict,
		},
		{
			Alias:                 "prod-web",
			User:                  "deploy",
			Host:                  "10.0.0.12",
			Port:                  2222,
			Auth:                  "key",
			ResolvedHostKeyPolicy: config.HostKeyPolicyAcceptNew,
			JumpHost:              "45.62.111.213",
			Tags:                  []string{"prod", "web"},
			Note:                  "主站应用服务",
		},
	}
}

func widestLine(s string) int {
	width := 0
	for line := range strings.SplitSeq(s, "\n") {
		width = max(width, displayWidth(line))
	}
	return width
}
