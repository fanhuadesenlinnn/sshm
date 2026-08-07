package command

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

func TestSearchMatchHost(t *testing.T) {
	host := config.Host{Alias: "web01", Host: "10.0.0.1", User: "root", Note: "生产环境", Tags: []string{"web", "prod"}}
	for _, keyword := range []string{"web01", "10.0", "root", "生产", "web", "prod"} {
		if !matchHost(host, keyword) {
			t.Fatalf("matchHost(%q) 应为 true", keyword)
		}
	}
	for _, keyword := range []string{"db", "staging", "other"} {
		if matchHost(host, keyword) {
			t.Fatalf("matchHost(%q) 应为 false", keyword)
		}
	}
}

func TestSearchWithoutArgsRequiresTerminal(t *testing.T) {
	if ui.IsTerminal() {
		t.Skip("需要在非终端环境验证")
	}
	app := &App{}
	if err := app.cmdSearch(nil); err == nil || !strings.Contains(err.Error(), "sshm search") {
		t.Fatalf("非交互无关键词应报用法: %v", err)
	}
}

func TestSearchMatchHostTerms(t *testing.T) {
	host := config.Host{Alias: "web01", Host: "10.0.0.1", Tags: []string{"web"}}
	if !matchHostTerms(host, []string{"tag:web", "10.0"}) {
		t.Fatal("多词 AND 匹配应命中")
	}
	if matchHostTerms(host, []string{"tag:web", "db"}) {
		t.Fatal("任一条件不满足应整体不命中")
	}
	if matchHostTerms(host, []string{"tag:db"}) {
		t.Fatal("tag: 前缀应限定标签匹配")
	}
}

func TestHostLookupErrorSuggestsCloseAlias(t *testing.T) {
	store := config.NewStoreWithPath(filepath.Join(t.TempDir(), "sshm.yaml"))
	initCommandTestStore(t, store)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host = "web01", "root", "10.0.0.1"
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: store.Path()}
	err := app.hostLookupError("web2", errors.New("cause"))
	if err == nil {
		t.Fatal("hostLookupError 应返回错误")
	}
	if !strings.Contains(err.Error(), "你是否想使用") {
		t.Fatalf("错误应包含建议: %v", err)
	}
}
