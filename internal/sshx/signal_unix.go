//go:build !windows

package sshx

import (
	"os"
	"syscall"
)

func syscallSigwinch() os.Signal {
	return syscall.SIGWINCH
}
