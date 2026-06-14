package main

import (
	"fmt"
	"os"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/command"
)

func main() {
	args := os.Args[1:]

	if err := command.Run(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(command.ExitCodeForError(err))
	}
}
