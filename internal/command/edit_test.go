package command

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func TestEditHostWithBlankInputsDoesNotRewriteConfig(t *testing.T) {
	app := newEditTestApp(t)
	before, err := os.ReadFile(app.Store.Path())
	if err != nil {
		t.Fatal(err)
	}

	withCommandStdin(t, strings.Repeat("\n", 11), func() {
		if err := app.cmdEdit([]string{"server"}); err != nil {
			t.Fatal(err)
		}
	})

	after, err := os.ReadFile(app.Store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("blank edit rewrote sshm.yaml")
	}
}

func TestEditHostChangesOnlyEnteredField(t *testing.T) {
	app := newEditTestApp(t)
	before := loadEditTestHost(t, app)

	input := strings.Join([]string{
		"", "", "", "", "",
		"updated note",
		"", "", "", "", "",
	}, "\n") + "\n"
	withCommandStdin(t, input, func() {
		if err := app.cmdEdit([]string{"server"}); err != nil {
			t.Fatal(err)
		}
	})

	after := loadEditTestHost(t, app)
	want := before
	want.Note = "updated note"
	if !reflect.DeepEqual(after, want) {
		t.Fatalf("host after edit = %+v, want %+v", after, want)
	}
}

func TestEditHostReplacesRequiredFieldInsteadOfAppending(t *testing.T) {
	app := newEditTestApp(t)
	before := loadEditTestHost(t, app)

	input := "renamed\n" + strings.Repeat("\n", 10)
	withCommandStdin(t, input, func() {
		if err := app.cmdEdit([]string{"server"}); err != nil {
			t.Fatal(err)
		}
	})

	after := loadEditTestHost(t, app)
	want := before
	want.Alias = "renamed"
	if !reflect.DeepEqual(after, want) {
		t.Fatalf("host after edit = %+v, want %+v", after, want)
	}
}

func TestHostEditPatchOnlyAppliesSpecifiedFields(t *testing.T) {
	host := config.Host{
		ID: "stable", Alias: "server", User: "root", Host: "example.com", Port: 22,
		Identity: "managed:personal", Note: "old", Tags: []string{"prod"}, Auth: "auto",
		PasswordRef: "stable", Pinned: true,
		LastUsedAt: "2026-06-14T00:00:00Z", HostKeyPolicy: config.HostKeyPolicyAcceptNew,
	}
	want := host
	want.Note = "new"
	note := "new"
	patch := hostEditPatch{note: &note}
	if !patch.apply(&host) {
		t.Fatal("patch should report a change")
	}
	if !reflect.DeepEqual(host, want) {
		t.Fatalf("patch changed unspecified fields: %+v, want %+v", host, want)
	}
	if patch.apply(&host) {
		t.Fatal("applying the same patch should be a no-op")
	}
}

func newEditTestApp(t *testing.T) *App {
	t.Helper()
	store := config.NewStoreWithPath(filepath.Join(t.TempDir(), "sshm.yaml"))
	initCommandTestStore(t, store)
	host := config.Host{
		ID:            config.NewID(),
		Alias:         "server",
		User:          "root",
		Host:          "example.com",
		Port:          22,
		Note:          "old note",
		Tags:          []string{"prod", "linux"},
		Auth:          "auto",
		Pinned:        true,
		LastUsedAt:    "2026-06-14T00:00:00Z",
		HostKeyPolicy: config.HostKeyPolicyAcceptNew,
	}
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	return &App{Store: store, ConfigPath: store.Path()}
}

func loadEditTestHost(t *testing.T, app *App) config.Host {
	t.Helper()
	doc, err := app.Store.Repository().Load()
	if err != nil {
		t.Fatal(err)
	}
	return doc.Hosts[0]
}

func withCommandStdin(t *testing.T, input string, run func()) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	original := os.Stdin
	os.Stdin = reader
	defer func() {
		os.Stdin = original
		_ = reader.Close()
	}()
	run()
}
