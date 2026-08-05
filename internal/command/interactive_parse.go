package command

import (
	"fmt"
	"strings"
)

type inputToken struct {
	Value string
	Start int
	End   int
}

var interactiveExecFlags = map[string]bool{
	"--yes":    true,
	"--quiet":  true,
	"--no-log": true,
}

var interactiveBatchFlags = map[string]bool{
	"--fail-fast": true,
	"--yes":       true,
	"--no-log":    true,
	"--quiet":     true,
}

var interactiveBatchValueFlags = map[string]bool{
	"--parallel":         true,
	"--serial":           true,
	"--timeout":          true,
	"--connect-timeout":  true,
	"--max-fail":         true,
	"--max-fail-percent": true,
}

// parseInteractiveInput separates sshm's local routing prefix from an exec
// payload. Remote commands are kept as one argument after -- so their quotes,
// escapes, variables, and spacing are not parsed and reconstructed locally.
func parseInteractiveInput(input string) ([]string, error) {
	first, _, ok, err := nextInputToken(input, 0)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	switch strings.ToLower(first.Value) {
	case "exec", "x":
		return parseInteractiveRemoteInput(input, first, interactiveExecFlags, nil)
	case "exec-tag", "xt":
		return parseInteractiveRemoteInput(input, first, interactiveBatchFlags, interactiveBatchValueFlags)
	default:
		return parseArgs(input)
	}
}

func parseInteractiveRemoteInput(
	input string,
	commandToken inputToken,
	booleanFlags map[string]bool,
	valueFlags map[string]bool,
) ([]string, error) {
	localArgs := []string{}
	target := ""
	offset := commandToken.End

	for {
		token, next, ok, err := nextInputToken(input, offset)
		if err != nil {
			return nil, err
		}
		if !ok {
			parts := []string{commandToken.Value}
			parts = append(parts, localArgs...)
			if target != "" {
				parts = append(parts, target)
			}
			return parts, nil
		}

		if token.Value == "--" {
			if target == "" {
				return nil, fmt.Errorf("%s 的 -- 前缺少目标", commandToken.Value)
			}
			commandStart := skipInputSpaces(input, token.End)
			if commandStart >= len(input) {
				return nil, fmt.Errorf("%s 的 -- 后缺少远程命令", commandToken.Value)
			}
			return buildInteractiveRemoteArgs(commandToken.Value, localArgs, target, input[commandStart:]), nil
		}

		if booleanFlags[token.Value] {
			localArgs = append(localArgs, token.Value)
			offset = next
			continue
		}
		if valueFlags[token.Value] {
			value, valueNext, valueOK, valueErr := nextInputToken(input, next)
			if valueErr != nil {
				return nil, valueErr
			}
			if !valueOK || value.Value == "--" {
				return nil, fmt.Errorf("%s 缺少值", token.Value)
			}
			localArgs = append(localArgs, token.Value, value.Value)
			offset = valueNext
			continue
		}
		if strings.HasPrefix(token.Value, "-") {
			return nil, fmt.Errorf("未知 sshm 选项 %s；如果它属于远程命令，请在前面加 --", token.Value)
		}
		if target == "" {
			target = token.Value
			offset = next
			continue
		}

		command := legacyWrappedCommand(input[token.Start:])
		return buildInteractiveRemoteArgs(commandToken.Value, localArgs, target, command), nil
	}
}

func buildInteractiveRemoteArgs(command string, localArgs []string, target, remoteCommand string) []string {
	parts := make([]string, 0, 4+len(localArgs))
	parts = append(parts, command)
	parts = append(parts, localArgs...)
	parts = append(parts, target, "--", remoteCommand)
	return parts
}

// legacyWrappedCommand preserves compatibility with the old interactive form
// `x host 'complete command'`. Only a single quote-wrapped token covering the
// entire payload is unwrapped; quotes inside a normal command remain untouched.
func legacyWrappedCommand(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) < 2 || (trimmed[0] != '\'' && trimmed[0] != '"') {
		return raw
	}
	token, next, ok, err := nextInputToken(trimmed, 0)
	if err == nil && ok && skipInputSpaces(trimmed, next) == len(trimmed) && token.End == len(trimmed) {
		return token.Value
	}
	return raw
}

// parseArgs tokenizes sshm management commands without performing shell
// expansion. It supports quoting, escaped characters, and empty quoted values,
// and reports malformed input instead of silently changing it.
func parseArgs(input string) ([]string, error) {
	var args []string
	offset := 0
	for {
		token, next, ok, err := nextInputToken(input, offset)
		if err != nil {
			return nil, err
		}
		if !ok {
			return args, nil
		}
		args = append(args, token.Value)
		offset = next
	}
}

func nextInputToken(input string, offset int) (inputToken, int, bool, error) {
	i := skipInputSpaces(input, offset)
	if i >= len(input) {
		return inputToken{}, i, false, nil
	}

	start := i
	quote := byte(0)
	started := false
	var value strings.Builder
	for i < len(input) {
		ch := input[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
				started = true
				i++
				continue
			}
			if quote == '"' && ch == '\\' {
				if i+1 < len(input) && input[i+1] == '"' {
					value.WriteByte('"')
					i += 2
					started = true
					continue
				}
				value.WriteByte(ch)
				i++
				started = true
				continue
			}
			value.WriteByte(ch)
			started = true
			i++
			continue
		}

		switch ch {
		case ' ', '\t':
			if started {
				return inputToken{Value: value.String(), Start: start, End: i}, i, true, nil
			}
		case '\'', '"':
			quote = ch
			started = true
			i++
			continue
		case '\\':
			if i+1 < len(input) {
				next := input[i+1]
				if next == ' ' || next == '\t' || next == '\'' || next == '"' {
					value.WriteByte(next)
					started = true
					i += 2
					continue
				}
			}
			value.WriteByte(ch)
			started = true
			i++
			continue
		default:
			value.WriteByte(ch)
			started = true
			i++
			continue
		}
	}

	if quote != 0 {
		return inputToken{}, i, false, fmt.Errorf("输入中的 %c 引号未闭合", quote)
	}
	return inputToken{Value: value.String(), Start: start, End: i}, i, true, nil
}

func skipInputSpaces(input string, offset int) int {
	for offset < len(input) && (input[offset] == ' ' || input[offset] == '\t') {
		offset++
	}
	return offset
}
