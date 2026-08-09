// Package shellquote builds POSIX shell command lines with safe single
// quoting. Every argument is wrapped in single quotes, with embedded single
// quotes escaped safely so the result is always exactly one shell word.
package shellquote

import (
	"fmt"
	"strings"
)

// Single quotes a value for use as a single shell word.
func Single(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// Command quotes every argument and joins them with spaces, producing a
// command line where each argument is one shell word.
func Command(args []string) string {
	quoted := make([]string, len(args))
	for index, arg := range args {
		quoted[index] = Single(arg)
	}
	return strings.Join(quoted, " ")
}

// Split tokenizes a command into literal argv values without performing shell
// expansion. Quotes and backslashes group characters locally; operators such
// as $, ;, | and parentheses remain ordinary argument data.
func Split(input string) ([]string, error) {
	var args []string
	var value strings.Builder
	quote := byte(0)
	started := false
	flush := func() {
		args = append(args, value.String())
		value.Reset()
		started = false
	}
	for index := 0; index < len(input); index++ {
		ch := input[index]
		if quote != 0 {
			if ch == quote {
				quote = 0
				started = true
				continue
			}
			if quote == '"' && ch == '\\' {
				if index+1 >= len(input) {
					return nil, fmt.Errorf("命令以未完成的转义结尾")
				}
				next := input[index+1]
				if next == '"' || next == '\\' || next == '$' || next == '`' || next == '\n' {
					index++
					if next != '\n' {
						value.WriteByte(next)
					}
				} else {
					value.WriteByte('\\')
				}
				started = true
				continue
			}
			value.WriteByte(ch)
			started = true
			continue
		}
		switch ch {
		case ' ', '\t', '\r', '\n':
			if started {
				flush()
			}
		case '\'', '"':
			quote = ch
			started = true
		case '\\':
			if index+1 >= len(input) {
				return nil, fmt.Errorf("命令以未完成的转义结尾")
			}
			index++
			value.WriteByte(input[index])
			started = true
		default:
			value.WriteByte(ch)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("命令中的 %c 引号未闭合", quote)
	}
	if started {
		flush()
	}
	return args, nil
}
