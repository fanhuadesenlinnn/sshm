//go:build !windows

package ui

import (
	"golang.org/x/term"
	"os"
)

func readPasswordFromTTY() (string, error) {
	pass, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	return string(pass), nil
}
