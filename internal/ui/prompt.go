package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var reader = bufio.NewReader(os.Stdin)

// ReadLine reads a line from stdin after printing a prompt.
func ReadLine(prompt string) string {
	fmt.Print(prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimRight(input, "\r\n")
}

// ReadLineDefault reads a line with a default value.
func ReadLineDefault(prompt string, defaultVal string) string {
	fmt.Print(prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultVal
	}
	input = strings.TrimRight(input, "\r\n")
	if input == "" {
		return defaultVal
	}
	return input
}

// ReadYesNo reads a yes/no answer (default no).
func ReadYesNo(prompt string) bool {
	input := ReadLine(prompt)
	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes"
}

// ReadPassword reads a password without echoing.
func ReadPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	// Use terminal to read password
	pass, err := readPasswordFromTTY()
	if err != nil {
		return "", err
	}
	fmt.Println()
	return pass, nil
}

// IsTerminal returns true if stdin is a terminal.
func IsTerminal() bool {
	return isTerminal()
}
