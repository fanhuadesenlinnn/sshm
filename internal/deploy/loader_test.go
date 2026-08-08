package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
)

func TestGlobalInitializeCreatesLoadableEmptyV3Starter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SSHMD_HOME", home)
	if _, _, err := config.Initialize(false); err != nil {
		t.Fatal(err)
	}
	paths, err := Discover(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != filepath.Join(home, "deploy.yaml") {
		t.Fatalf("discovered starter paths = %v", paths)
	}
	catalog, err := Load(paths)
	if err != nil {
		t.Fatalf("generated v3 starter should strictly load: %v", err)
	}
	if err := ValidateCatalog(catalog); err != nil {
		t.Fatalf("generated v3 starter should validate: %v", err)
	}
	if len(catalog.Plays) != 0 {
		t.Fatalf("generated starter must not contain active plays: %+v", catalog.Plays)
	}
	if _, err := Load([]string{filepath.Join(home, "README.md")}); err == nil {
		t.Fatal("README 不应被当作 deploy 文件加载")
	}
}

func TestLoadRejectsRemovedV2Files(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.yaml")
	if err := os.WriteFile(path, []byte("version: 2\nprofiles: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Load([]string{path})
	if err == nil || !strings.Contains(err.Error(), "Deploy v2") {
		t.Fatalf("v2 文件应被明确拒绝: %v", err)
	}
}

func TestLoadParsesTaskLevelConfirmField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.yaml")
	source := `
version: 3
plays:
  - name: gate
    hosts: { hosts: [web01] }
    tasks:
      - name: 上传
        copy:
          content: v1
          dest: /tmp/gate.txt
        confirm: 确认继续?
        register: up
`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := Load([]string{path})
	if err != nil {
		t.Fatalf("任务级 confirm 应能解析: %v", err)
	}
	tasks := catalog.Plays[0].Tasks
	if tasks[0].Confirm != "确认继续?" || tasks[0].Module != "copy" {
		t.Fatalf("模块任务携带 confirm 解析错误: %+v", tasks[0])
	}
	if err := ValidateCatalog(catalog); err != nil {
		t.Fatalf("带 confirm 的 playbook 应通过校验: %v", err)
	}
	confirmOnly := &Catalog{ByName: map[string]Play{}, Sources: catalog.Sources}
	play := catalog.Plays[0]
	play.Tasks = []Task{{Name: "纯门禁", Confirm: "确认继续?"}}
	confirmOnly.Plays = []Play{play}
	confirmOnly.ByName[play.Name] = play
	if err := ValidateCatalog(confirmOnly); err == nil || !strings.Contains(err.Error(), "搭配") {
		t.Fatalf("confirm-only 任务应报出搭配提示: %v", err)
	}
}

func TestExpandIncludesNestedAndDetectsCycles(t *testing.T) {
	dir := t.TempDir()
	tasksDir := filepath.Join(dir, "tasks")
	if err := os.MkdirAll(tasksDir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(tasksDir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.yaml", "tasks:\n  - include: ./b.yaml\n  - name: a\n    command:\n      cmd: echo a\n")
	write("b.yaml", "tasks:\n  - name: b\n    command:\n      cmd: echo b\n")
	write("cycle-a.yaml", "tasks:\n  - include: ./cycle-b.yaml\n")
	write("cycle-b.yaml", "tasks:\n  - include: ./cycle-a.yaml\n")

	expanded, err := expandIncludes(
		[]Task{{Include: "./tasks/a.yaml"}},
		dir, nil, 0,
	)
	if err != nil {
		t.Fatalf("嵌套 include 应展开: %v", err)
	}
	if len(expanded) != 2 || expanded[0].Name != "b" || expanded[1].Name != "a" {
		t.Fatalf("嵌套展开顺序错误: %+v", expanded)
	}
	if expanded[0].BaseDir != tasksDir {
		t.Fatalf("片段任务 BaseDir 应为片段目录: %s", expanded[0].BaseDir)
	}

	_, err = expandIncludes(
		[]Task{{Include: "./tasks/cycle-a.yaml"}},
		dir, nil, 0,
	)
	if err == nil || !strings.Contains(err.Error(), "include 循环") {
		t.Fatalf("include 循环应被检测: %v", err)
	}
}
