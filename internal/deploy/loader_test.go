package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func TestGlobalInitializeCreatesLoadableEmptyV3Starter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SSHM_HOME", home)
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
