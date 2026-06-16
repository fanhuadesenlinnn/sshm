package ui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// StderrIsTerminal reports whether stderr is an interactive terminal.
// Batch progress uses this to decide whether to draw an in-place counter or
// stay silent for redirected stderr.
func StderrIsTerminal() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// RefreshLine rewrites the current stderr line in place when stderr is a
// terminal (a live progress counter). When stderr is not a terminal (piped to
// a file, CI logs, ...), it does nothing so captured output is not smeared
// with half-finished progress fragments. Callers should follow a finished
// batch with EndProgress so the terminal cursor advances to a fresh line.
func RefreshLine(format string, args ...interface{}) {
	if StderrIsTerminal() {
		fmt.Fprintf(os.Stderr, "\r\033[K"+format, args...)
	}
}

// EndProgress advances the stderr cursor to a new line after in-place progress.
// It is a no-op when stderr is not a terminal, so log output stays clean.
func EndProgress() {
	if StderrIsTerminal() {
		fmt.Fprintln(os.Stderr)
	}
}
