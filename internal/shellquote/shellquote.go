// Package shellquote builds POSIX shell command lines with safe single
// quoting. Every argument is wrapped in single quotes, with embedded single
// quotes escaped as '\” so the result is always exactly one shell word.
package shellquote

import "strings"

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
