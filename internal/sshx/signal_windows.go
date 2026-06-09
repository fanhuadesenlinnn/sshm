//go:build windows

package sshx

import "os"

func syscallSigwinch() os.Signal {
	// SIGWINCH is not available on Windows, return nil
	return nil
}
