//go:build !windows

package safefile

import (
	"os"
)

// Restrict applies the private permissions used for sshmd-owned state.
func Restrict(path string, perm os.FileMode) error {
	return os.Chmod(path, perm)
}
