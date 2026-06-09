//go:build windows

package ui

import (
	"fmt"
	"golang.org/x/term"
	"os"
)

func readPasswordFromTTY() (string, error) {
	// On Windows, fall back to simple line reading
	var pass string
	fmt.Scanln(&pass)
	return pass, nil
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
