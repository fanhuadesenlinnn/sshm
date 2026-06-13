package sshx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
)

// ConnectOpenSSHKey attempts a key-only SSH connection using the system ssh binary.
func ConnectOpenSSHKey(h config.Host, extraArgs []string) int {
	return runInteractive(buildConnectArgs(h, extraArgs, true))
}

// ConnectOpenSSHDefault calls system ssh without overriding its authentication behavior.
func ConnectOpenSSHDefault(h config.Host, extraArgs []string) int {
	return runInteractive(buildConnectArgs(h, extraArgs, false))
}

func buildConnectArgs(h config.Host, extraArgs []string, keyOnly bool) []string {
	args := []string{"ssh", "-p", fmt.Sprintf("%d", h.Port)}
	if keyOnly {
		args = append(args, identityArgs(h)...)
		args = append(args,
			"-o", "PreferredAuthentications=publickey",
			"-o", "PasswordAuthentication=no",
			"-o", "StrictHostKeyChecking=ask",
		)
	}
	args = append(args, extraArgs...)
	return append(args, fmt.Sprintf("%s@%s", h.User, h.Host))
}

// ConnectionCommand returns a copyable system SSH command for a host.
func ConnectionCommand(h config.Host) string {
	args := []string{"ssh", "-p", fmt.Sprintf("%d", h.Port)}
	strategy := GetAuthStrategy(h.Auth)
	if h.Identity != "" && (strategy == AuthAuto || strategy == AuthKey) {
		args = append(args, "-i", config.ExpandPath(h.Identity), "-o", "IdentitiesOnly=yes")
	}
	args = append(args, fmt.Sprintf("%s@%s", h.User, h.Host))
	for i := range args {
		args[i] = shellQuote(args[i])
	}
	return strings.Join(args, " ")
}

func shellQuote(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n'\"\\$`!&|;<>()[]{}*?~") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func runInteractive(args []string) int {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return exitCode(cmd.Run())
}

// ExecOpenSSH runs a key-only command using the configured identity.
func ExecOpenSSH(h config.Host, command string) (string, error) {
	return ExecOpenSSHContext(context.Background(), h, command)
}

// ExecOpenSSHDefault runs a command using system SSH defaults.
func ExecOpenSSHDefault(h config.Host, command string) (string, error) {
	return ExecOpenSSHDefaultContext(context.Background(), h, command)
}

func ExecOpenSSHContext(ctx context.Context, h config.Host, command string) (string, error) {
	return runCombinedContext(ctx, buildExecArgs(h, command, true))
}

func ExecOpenSSHDefaultContext(ctx context.Context, h config.Host, command string) (string, error) {
	return runCombinedContext(ctx, buildExecArgs(h, command, false))
}

func buildExecArgs(h config.Host, command string, keyOnly bool) []string {
	args := []string{"ssh", "-p", fmt.Sprintf("%d", h.Port)}
	if keyOnly {
		args = append(args, identityArgs(h)...)
		args = append(args,
			"-o", "PreferredAuthentications=publickey",
			"-o", "PasswordAuthentication=no",
			"-o", "StrictHostKeyChecking=yes",
			"-o", "BatchMode=yes",
		)
	}
	args = append(args, fmt.Sprintf("%s@%s", h.User, h.Host), command)
	return args
}

// Ping checks connectivity with the configured identity.
func Ping(h config.Host) (bool, string) {
	return runPing(buildPingArgs(h, true))
}

// PingDefault checks connectivity using system SSH defaults.
func PingDefault(h config.Host) (bool, string) {
	return runPing(buildPingArgs(h, false))
}

func buildPingArgs(h config.Host, keyOnly bool) []string {
	args := []string{
		"ssh", "-p", fmt.Sprintf("%d", h.Port),
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=yes",
	}
	if keyOnly {
		args = append(args, identityArgs(h)...)
		args = append(args,
			"-o", "PreferredAuthentications=publickey",
			"-o", "PasswordAuthentication=no",
		)
	}
	args = append(args, fmt.Sprintf("%s@%s", h.User, h.Host), "echo ok")
	return args
}

func identityArgs(h config.Host) []string {
	return []string{"-i", config.ExpandPath(h.Identity), "-o", "IdentitiesOnly=yes"}
}

func runCombined(args []string) (string, error) {
	return runCombinedContext(context.Background(), args)
}

func runCombinedContext(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(output), fmt.Errorf("SSH 命令超时或取消: %w", ctx.Err())
	}
	return string(output), err
}

func runPing(args []string) (bool, string) {
	output, err := runCombined(args)
	return err == nil, output
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus()
		}
	}
	return 1
}

// HasIdentity returns true if the host has a configured identity file that exists.
func HasIdentity(h config.Host) bool {
	if h.Identity == "" {
		return false
	}
	_, err := os.Stat(config.ExpandPath(h.Identity))
	return err == nil
}
