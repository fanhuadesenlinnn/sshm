package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
)

func TestTargetSelectorStringAndEmpty(t *testing.T) {
	if !(TargetSelector{}).Empty() {
		t.Fatal("空选择器应为 Empty")
	}
	if (TargetSelector{All: true}).Empty() {
		t.Fatal("all 不应为 Empty")
	}
	cases := []struct {
		selector TargetSelector
		want     string
	}{
		{TargetSelector{All: true}, "all"},
		{TargetSelector{Hosts: []string{"a", "b"}}, "host:a,b"},
		{TargetSelector{Tags: []string{"web", "db"}}, "tag:web,db"},
		{TargetSelector{Hosts: []string{"a"}, Tags: []string{"web"}}, "host:a + tag:web"},
	}
	for _, tc := range cases {
		if got := tc.selector.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

func TestConditionMatches(t *testing.T) {
	if !(Condition{RCIn: []int{0, 2}}).Matches(2) {
		t.Fatal("rc_in 应匹配")
	}
	if (Condition{RCIn: []int{0}}).Matches(1) {
		t.Fatal("rc_in 不应误匹配")
	}
	if (Condition{RCNotIn: []int{1}}).Matches(1) {
		t.Fatal("rc_not_in 不应匹配")
	}
	if !(Condition{RCNotIn: []int{1}}).Matches(2) {
		t.Fatal("rc_not_in 非排除值应匹配")
	}
	if (Condition{}).Matches(1) {
		t.Fatal("空条件不应匹配")
	}
}

func TestResolveTargets(t *testing.T) {
	hosts := []config.Host{
		{Alias: "web01", ID: "id-1", Tags: []string{"web", "prod"}},
		{Alias: "db01", ID: "id-2", Tags: []string{"db"}},
		{Alias: "lb01", ID: "id-3", Tags: []string{"web"}},
	}
	byAlias, err := ResolveTargets(hosts, TargetSelector{Hosts: []string{"web01,db01"}})
	if err != nil || len(byAlias) != 2 || byAlias[0].Alias != "web01" || byAlias[1].Alias != "db01" {
		t.Fatalf("byAlias = %+v, err = %v", byAlias, err)
	}
	byID, err := ResolveTargets(hosts, TargetSelector{Hosts: []string{"id-3"}})
	if err != nil || len(byID) != 1 || byID[0].Alias != "lb01" {
		t.Fatalf("byID = %+v, err = %v", byID, err)
	}
	byTag, err := ResolveTargets(hosts, TargetSelector{Tags: []string{"web"}})
	if err != nil || len(byTag) != 2 {
		t.Fatalf("byTag = %+v, err = %v", byTag, err)
	}
	all, err := ResolveTargets(hosts, TargetSelector{All: true})
	if err != nil || len(all) != 3 {
		t.Fatalf("all = %+v, err = %v", all, err)
	}
	if _, err := ResolveTargets(hosts, TargetSelector{}); err == nil {
		t.Fatal("空选择器应报错")
	}
	if _, err := ResolveTargets(hosts, TargetSelector{Hosts: []string{"missing"}}); err == nil {
		t.Fatal("不存在的主机应报错")
	}
	if _, err := ResolveTargets(hosts, TargetSelector{Tags: []string{"nope"}}); err == nil {
		t.Fatal("不存在的标签应报错")
	}
	if _, err := ResolveTargets(hosts, TargetSelector{All: true, Hosts: []string{"web01"}}); err == nil {
		t.Fatal("all 混用 hosts 应报错")
	}
}

func TestResolveTargetsWithExcludes(t *testing.T) {
	hosts := []config.Host{
		{Alias: "web01", ID: "id-1", Tags: []string{"web", "prod"}},
		{Alias: "web02", ID: "id-2", Tags: []string{"web", "legacy"}},
		{Alias: "db01", ID: "id-3", Tags: []string{"db"}},
	}
	byHost, err := ResolveTargets(hosts, TargetSelector{Hosts: []string{"web01,web02"}, Exclude: []string{"web02"}})
	if err != nil || len(byHost) != 1 || byHost[0].Alias != "web01" {
		t.Fatalf("按主机排除 = %+v, err = %v", byHost, err)
	}
	byTag, err := ResolveTargets(hosts, TargetSelector{Tags: []string{"web"}, ExcludeTags: []string{"legacy"}})
	if err != nil || len(byTag) != 1 || byTag[0].Alias != "web01" {
		t.Fatalf("按标签排除 = %+v, err = %v", byTag, err)
	}
	all, err := ResolveTargets(hosts, TargetSelector{All: true, Exclude: []string{"id-3"}})
	if err != nil || len(all) != 2 {
		t.Fatalf("all+排除 = %+v, err = %v", all, err)
	}
}

func TestApplyExcludesValidation(t *testing.T) {
	inventory := []config.Host{
		{Alias: "web01", ID: "id-1", Tags: []string{"web"}},
		{Alias: "db01", ID: "id-2", Tags: []string{"db"}},
	}
	if _, err := ApplyExcludes(inventory, inventory, []string{"nope"}, nil); err == nil {
		t.Fatal("排除不存在的主机应报错")
	}
	if _, err := ApplyExcludes(inventory, inventory, nil, []string{"nope"}); err == nil {
		t.Fatal("排除不存在的标签应报错")
	}
	if _, err := ApplyExcludes(inventory, inventory, []string{"web01", "id-2"}, nil); err == nil {
		t.Fatal("全部排除后应报错")
	}
	if _, err := ApplyExcludes(inventory, inventory, nil, []string{"web", "db"}); err == nil {
		t.Fatal("标签全部排除后应报错")
	}
}

func TestBuildPlanMergesExcludeOnlyOverrideWithPlayHosts(t *testing.T) {
	hosts := testHostsFixture()
	play := Play{
		Name:  "web",
		Hosts: TargetSelector{Hosts: []string{"web01", "web02"}},
		Tasks: []Task{commandTaskFixture("echo ok")},
	}
	catalog := &Catalog{ByName: map[string]Play{"web": play}}

	plan, err := BuildPlan(catalog, "web", hosts, Overrides{Targets: &TargetSelector{Exclude: []string{"web02"}}})
	if err != nil {
		t.Fatalf("exclude-only 覆盖应保留 play hosts: %v", err)
	}
	if len(plan.Hosts) != 1 || plan.Hosts[0].Alias != "web01" {
		t.Fatalf("排除后主机 = %+v", plan.Hosts)
	}

	_, err = BuildPlan(catalog, "web", hosts, Overrides{Targets: &TargetSelector{Exclude: []string{"web01", "web02"}}})
	if err == nil || !strings.Contains(err.Error(), "排除后目标主机为空") {
		t.Fatalf("排除全部主机应报明确错误: %v", err)
	}
}

func testHostsFixture() []config.Host {
	return []config.Host{
		{Alias: "web01", ID: "id-1", Host: "10.0.0.1", User: "root", Port: 22, Tags: []string{"web"}},
		{Alias: "web02", ID: "id-2", Host: "10.0.0.2", User: "root", Port: 22, Tags: []string{"web"}},
	}
}

func commandTaskFixture(cmd string) Task {
	return Task{Module: "command", Args: argsNode(map[string]any{"cmd": cmd})}
}

func TestDiscoverExplicitDeduplicatesAndAbsolutizes(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.yaml")
	second := filepath.Join(dir, "b.yaml")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("version: 3\nplays: []\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := Discover([]string{first, second, first})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != first || paths[1] != second {
		t.Fatalf("paths = %v", paths)
	}
}

func TestDiscoverDefaultUsesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SSHMD_HOME", home)
	if err := os.MkdirAll(config.DeployDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.DeployFilePath(), []byte("version: 3\nplays: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(nil); err != nil {
		t.Fatalf("默认发现失败: %v", err)
	}
	if err := os.Remove(config.DeployFilePath()); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(nil); err == nil {
		t.Fatal("无配置时应报错")
	}
}
