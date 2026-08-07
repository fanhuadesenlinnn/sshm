package deployv3

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateFromV2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.yaml")
	source := `
version: 2
profiles:
  - name: update-app
    targets:
      tags: [prod]
    serial: 2
    parallel: 2
    steps:
      - name: 创建目录
        mkdir:
          path: /opt/app/releases
          mode: "0755"
      - name: 上传应用包
        push:
          src: ./dist/app.tar.gz
          dest: /opt/app/releases/app.tar.gz
          backup: true
        notify: [重启应用]
      - name: 重启
        exec: systemctl restart app
        become: true
handlers:
  - name: 重启应用
    exec: systemctl restart app
`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	migration, err := MigrateFromV2([]string{path})
	if err != nil {
		t.Fatalf("MigrateFromV2: %v", err)
	}
	output := string(migration.YAML)
	for _, want := range []string{"version: 3", "plays:", "hosts:", "serial: 2", "state: directory", "copy:", "cmd: systemctl restart app"} {
		if !strings.Contains(output, want) {
			t.Errorf("迁移输出缺少 %q:\n%s", want, output)
		}
	}
	if len(migration.Warnings) == 0 {
		t.Error("notify/handler 迁移应当产生警告")
	}
}

func TestMigrateRejectsNotifyInV3(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.yaml")
	source := `
version: 3
plays:
  - name: demo
    hosts:
      hosts: [web01]
    tasks:
      - name: upload
        copy:
          src: ./a
          dest: /tmp/a
        notify: [restart]
`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load([]string{path}); err == nil || !strings.Contains(err.Error(), "notify") {
		t.Fatalf("v3 notify 应当报错，got %v", err)
	}
}
