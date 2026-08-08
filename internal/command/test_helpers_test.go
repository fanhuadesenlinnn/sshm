package command

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
)

func initCommandTestStore(t testing.TB, store *config.Store) {
	t.Helper()
	if err := store.Repository().Replace(config.DefaultDocument()); err != nil {
		t.Fatal(err)
	}
}

func captureStdout(t testing.TB, fn func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = original
	}()

	fn()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, reader); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.String()
}
