//go:build windows

package ui

import (
	"fmt"
)

func readPasswordFromTTY() (string, error) {
	// On Windows, fall back to simple line reading
	var pass string
	fmt.Scanln(&pass)
	return pass, nil
}
