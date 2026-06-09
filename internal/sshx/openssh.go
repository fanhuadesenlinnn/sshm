package sshx

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/sshm/sshm/internal/config"
)

// ConnectOpenSSHKey attempts a key-only SSH connection using the system ssh binary.
// Returns the exit code. 0 = success, non-zero = failure.
func ConnectOpenSSHKey(h config.Host, extraArgs []string) int {
	identityPath := config.ExpandPath(h.Identity)

	args := []string{
		"ssh",
		"-p", fmt.Sprintf("%d", h.Port),
		"-i", identityPath,
		"-o", "IdentitiesOnly=yes",
		"-o", "PreferredAuthentications=publickey",
		"-o", "PasswordAuthentication=no",
		"-o", "StrictHostKeyChecking=ask",
	}
	args = append(args, extraArgs...)
	args = append(args, fmt.Sprintf("%s@%s", h.User, h.Host))

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				return status.ExitStatus()
			}
		}
		return 1
	}
	return 0
}

// ConnectOpenSSHDefault calls system ssh with default authentication.
func ConnectOpenSSHDefault(h config.Host, extraArgs []string) int {
	args := []string{
		"ssh",
		"-p", fmt.Sprintf("%d", h.Port),
	}
	args = append(args, extraArgs...)
	args = append(args, fmt.Sprintf("%s@%s", h.User, h.Host))

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				return status.ExitStatus()
			}
		}
		return 1
	}
	return 0
}

// ExecOpenSSH runs a command on the remote host and returns output.
func ExecOpenSSH(h config.Host, command string) (string, error) {
	args := []string{
		"ssh",
		"-p", fmt.Sprintf("%d", h.Port),
		"-o", "StrictHostKeyChecking=accept-new",
		fmt.Sprintf("%s@%s", h.User, h.Host),
		command,
	}

	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// Ping checks if a host is reachable via SSH.
func Ping(h config.Host) (bool, string) {
	args := []string{
		"ssh",
		"-p", fmt.Sprintf("%d", h.Port),
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		fmt.Sprintf("%s@%s", h.User, h.Host),
		"echo ok",
	}

	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, string(output)
	}
	return true, string(output)
}

// HasIdentity returns true if the host has a configured identity file that exists.
func HasIdentity(h config.Host) bool {
	if h.Identity == "" {
		return false
	}
	path := config.ExpandPath(h.Identity)
	_, err := os.Stat(path)
	return err == nil
}
