//go:build windows

package ui

import (
	"golang.org/x/term"
	"os"
)

func readPasswordFromTTY() (string, error) {
	// term.ReadPassword works on Windows when stdin is a terminal
	pass, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	return string(pass), nil
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
