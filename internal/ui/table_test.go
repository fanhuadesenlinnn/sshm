package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
)

func TestDisplayConnectionTarget(t *testing.T) {
	host := config.Host{User: "root", Host: "10.0.0.1", Port: 22}
	if got := displayConnectionTarget(host); got != "root@10.0.0.1" {
		t.Fatalf("默认端口应省略: %q", got)
	}
	host.Port = 2222
	if got := displayConnectionTarget(host); got != "root@10.0.0.1:2222" {
		t.Fatalf("自定义端口 = %q", got)
	}
	host = config.Host{User: "deploy", Host: "2001:db8::1", Port: 22}
	if got := displayConnectionTarget(host); got != "deploy@[2001:db8::1]" {
		t.Fatalf("IPv6 应加括号: %q", got)
	}
}

func TestDisplayAliasAndTrust(t *testing.T) {
	if got := displayAlias(config.Host{Alias: "web", Pinned: true}); got != "* web" {
		t.Fatalf("pinned alias = %q", got)
	}
	host := config.Host{HostKeyPolicy: "accept-new"}
	if got := displayHostTrust(host); got != "accept-new" {
		t.Fatalf("trust = %q", got)
	}
	host.ResolvedHostKeyPolicy = "strict"
	host.HostKeyPolicy = "accept-new"
	if got := displayHostTrust(host); got != "strict" {
		t.Fatalf("resolved trust 应优先: %q", got)
	}
}

func TestPadAndTruncateToWidth(t *testing.T) {
	if got := padToWidth("ab", 4); got != "ab  " {
		t.Fatalf("pad = %q", got)
	}
	if got := truncateToWidth("abcdef", 3); got != "ab…" {
		t.Fatalf("truncate = %q", got)
	}
	if got := truncateToWidth("abc", 5); got != "abc" {
		t.Fatalf("短字符串不应截断: %q", got)
	}
}

func TestRenderHostsTableTo(t *testing.T) {
	var buffer bytes.Buffer
	hosts := []config.Host{
		{Alias: "web01", User: "root", Host: "10.0.0.1", Port: 22, Tags: []string{"web"}},
		{Alias: "db01", User: "deploy", Host: "10.0.0.2", Port: 5432, Tags: []string{"db"}},
	}
	renderHostsTableTo(&buffer, hosts, HostTableOptions{Width: 120, Wide: true})
	text := buffer.String()
	for _, want := range []string{"web01", "db01", "root@10.0.0.1", "deploy@10.0.0.2:5432", "连接目标", "标签"} {
		if !strings.Contains(text, want) {
			t.Fatalf("表格缺少 %q:\n%s", want, text)
		}
	}
}

func TestRenderHostsTableToEmpty(t *testing.T) {
	var buffer bytes.Buffer
	renderHostsTableTo(&buffer, nil, HostTableOptions{})
	if !strings.Contains(buffer.String(), "暂无主机") {
		t.Fatalf("空表格 = %q", buffer.String())
	}
}
