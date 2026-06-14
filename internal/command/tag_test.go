package command

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func TestTagLifecycleUpdatesDefinitionsAndReferences(t *testing.T) {
	app := newTagTestApp(t)

	if err := app.cmdTagCreate([]string{"audited", "--note", "已审计"}); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdTagEdit([]string{"audited", "--note", "每月审计"}); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdTagAdd([]string{"audited", "one", "two"}); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdTagRename([]string{"audited", "reviewed"}); err != nil {
		t.Fatal(err)
	}

	doc := loadTagTestDocument(t, app)
	tag, ok := doc.Tags.Find("reviewed")
	if !ok || tag.Note != "每月审计" {
		t.Fatalf("renamed tag = %+v, found = %t", tag, ok)
	}
	for _, alias := range []string{"one", "two"} {
		host := findTagTestHost(t, doc, alias)
		if !host.HasTag("reviewed") || host.HasTag("audited") {
			t.Fatalf("%s tags = %v", alias, host.Tags)
		}
	}

	if err := app.cmdTagDelete([]string{"reviewed"}); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("used tag deletion should require confirmation: %v", err)
	}
	if err := app.cmdTagDelete([]string{"reviewed", "--yes"}); err != nil {
		t.Fatal(err)
	}
	doc = loadTagTestDocument(t, app)
	if _, ok := doc.Tags.Find("reviewed"); ok {
		t.Fatal("deleted tag definition remains")
	}
	for _, host := range doc.Hosts {
		if host.HasTag("reviewed") {
			t.Fatalf("deleted tag remains on %s", host.Alias)
		}
	}
}

func TestTagBatchRelationOperations(t *testing.T) {
	app := newTagTestApp(t)

	if err := app.cmdTagAdd([]string{"audited", "--tag", "linux"}); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdTagAdd([]string{"audited", "--tag", "linux"}); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdTagRemove([]string{"linux", "one"}); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdTagSet([]string{"three", "--tags", "prod,audited"}); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdTagClear([]string{"two"}); err != nil {
		t.Fatal(err)
	}

	doc := loadTagTestDocument(t, app)
	if got := findTagTestHost(t, doc, "one").Tags; !reflect.DeepEqual(got, []string{"prod", "audited"}) {
		t.Fatalf("one tags = %v", got)
	}
	if got := findTagTestHost(t, doc, "two").Tags; len(got) != 0 {
		t.Fatalf("two tags = %v", got)
	}
	if got := findTagTestHost(t, doc, "three").Tags; !reflect.DeepEqual(got, []string{"prod", "audited"}) {
		t.Fatalf("three tags = %v", got)
	}
	for _, name := range []string{"audited", "linux", "prod"} {
		if _, ok := doc.Tags.Find(name); !ok {
			t.Fatalf("missing tag definition %q", name)
		}
	}
}

func TestTagCommandsRejectInvalidOrMissingInputs(t *testing.T) {
	app := newTagTestApp(t)
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "duplicate create", run: func() error { return app.cmdTagCreate([]string{"prod"}) }},
		{name: "invalid create", run: func() error { return app.cmdTagCreate([]string{"bad tag"}) }},
		{name: "missing remove tag", run: func() error { return app.cmdTagRemove([]string{"missing", "one"}) }},
		{name: "empty set", run: func() error { return app.cmdTagSet([]string{"one", "--tags", ""}) }},
		{name: "empty clear target", run: func() error { return app.cmdTagClear(nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func newTagTestApp(t *testing.T) *App {
	t.Helper()
	store := config.NewStoreWithPath(filepath.Join(t.TempDir(), "sshm.yaml"))
	for _, host := range []config.Host{
		{ID: config.NewID(), Alias: "one", User: "root", Host: "one", Port: 22, Auth: "auto", Tags: []string{"prod", "linux"}},
		{ID: config.NewID(), Alias: "two", User: "root", Host: "two", Port: 22, Auth: "auto", Tags: []string{"linux"}},
		{ID: config.NewID(), Alias: "three", User: "root", Host: "three", Port: 22, Auth: "auto"},
	} {
		if err := store.Add(host); err != nil {
			t.Fatal(err)
		}
	}
	return &App{Store: store, ConfigPath: store.Path()}
}

func loadTagTestDocument(t *testing.T, app *App) *config.Document {
	t.Helper()
	doc, err := app.Store.Repository().Load()
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func findTagTestHost(t *testing.T, doc *config.Document, alias string) *config.Host {
	t.Helper()
	for i := range doc.Hosts {
		if doc.Hosts[i].Alias == alias {
			return &doc.Hosts[i]
		}
	}
	t.Fatalf("missing host %s", alias)
	return nil
}
