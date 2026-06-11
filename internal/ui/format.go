package ui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// Colors
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
)

var noColor bool

func init() {
	noColor = os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(os.Stdout.Fd()))
}

// colorize wraps text in color codes if color is enabled.
func colorize(code, text string) string {
	if noColor {
		return text
	}
	return code + text + Reset
}

// Success formats a success/info message.
func Success(format string, args ...interface{}) string {
	return colorize(Green, ">> "+fmt.Sprintf(format, args...))
}

// Warn formats a warning message.
func Warn(format string, args ...interface{}) string {
	return colorize(Yellow, "!! "+fmt.Sprintf(format, args...))
}

// ErrorMsg formats an error message.
func ErrorMsg(format string, args ...interface{}) string {
	return colorize(Red, "Error: "+fmt.Sprintf(format, args...))
}

// Header formats a section header.
func Header(text string) string {
	return colorize(Cyan+Bold, text)
}

// BoldText returns bold text.
func BoldText(text string) string {
	return colorize(Bold, text)
}

// DimText returns dimmed text.
func DimText(text string) string {
	return colorize(Dim, text)
}

// Info formats a label-value pair with dimmed label.
func Info(label, value string) string {
	return colorize(Dim, label+": ") + value
}

// CyanText returns cyan text.
func CyanText(text string) string {
	return colorize(Cyan, text)
}

// PrintSuccess prints a success message with newline.
func PrintSuccess(format string, args ...interface{}) {
	fmt.Println(Success(format, args...))
}

// PrintWarn prints a warning message with newline.
func PrintWarn(format string, args ...interface{}) {
	fmt.Println(Warn(format, args...))
}

// PrintError prints an error message with newline.
func PrintError(format string, args ...interface{}) {
	fmt.Println(ErrorMsg(format, args...))
}

// PrintHeader prints a header with newline.
func PrintHeader(text string) {
	fmt.Println(Header(text))
}
