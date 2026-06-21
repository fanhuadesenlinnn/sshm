package command

import (
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func initCommandTestStore(t testing.TB, store *config.Store) {
	t.Helper()
	if err := store.Repository().Replace(config.DefaultDocument()); err != nil {
		t.Fatal(err)
	}
}
