package ui

import (
	"testing"
)

func TestVisualLength(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"", 0},
		{"你好", 4},
		{"a你好b", 6},
		{"日本語", 6},
		{"test123", 7},
		{"a b", 3},
	}

	for _, tt := range tests {
		got := visualLength(tt.input)
		if got != tt.want {
			t.Errorf("visualLength(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestDisplayWidth(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"你好世界", 8},
		{"abc你好", 7},
	}

	for _, tt := range tests {
		got := displayWidth(tt.input)
		if got != tt.want {
			t.Errorf("displayWidth(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestDisplayWidthWithANSI(t *testing.T) {
	colored := CyanText("hello")
	got := displayWidth(colored)
	if got != 5 {
		t.Errorf("displayWidth(cyan 'hello') = %d, want 5", got)
	}
}

func TestPadToWidth(t *testing.T) {
	tests := []struct {
		input string
		width int
		want  string
	}{
		{"hi", 5, "hi   "},
		{"hello", 5, "hello"},
		{"hello world", 5, "hell…"},
		{"你好", 6, "你好  "},
	}

	for _, tt := range tests {
		got := padToWidth(tt.input, tt.width)
		if got != tt.want {
			t.Errorf("padToWidth(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.want)
		}
	}
}

func TestTruncateToWidth(t *testing.T) {
	tests := []struct {
		input    string
		maxWidth int
		want     string
	}{
		{"hello", 3, "he…"},
		{"hello", 10, "hello"},
		{"你好世界", 3, "你…"},
		{"你好世界", 5, "你好…"},
		{"a", 1, "a"},
		{"ab", 1, ""},
	}

	for _, tt := range tests {
		got := truncateToWidth(tt.input, tt.maxWidth)
		if got != tt.want {
			t.Errorf("truncateToWidth(%q, %d) = %q, want %q", tt.input, tt.maxWidth, got, tt.want)
		}
	}
}

func TestRuneDisplayWidth(t *testing.T) {
	tests := []struct {
		r    rune
		want int
	}{
		{'a', 1},
		{'1', 1},
		{' ', 1},
		{'中', 2},
		{'あ', 2},
		{'한', 2},
	}

	for _, tt := range tests {
		got := runeDisplayWidth(tt.r)
		if got != tt.want {
			t.Errorf("runeDisplayWidth(%q) = %d, want %d", tt.r, got, tt.want)
		}
	}
}

func TestFormatFunctions(t *testing.T) {
	if Success("test") == "" {
		t.Fatal("Success() returned empty")
	}
	if Warn("test") == "" {
		t.Fatal("Warn() returned empty")
	}
	if ErrorMsg("test") == "" {
		t.Fatal("ErrorMsg() returned empty")
	}
	if Header("test") == "" {
		t.Fatal("Header() returned empty")
	}
}

func TestNoColor(t *testing.T) {
	origNoColor := noColor
	noColor = true
	defer func() { noColor = origNoColor }()

	if Success("test") != ">> test" {
		t.Errorf("Success() in no-color mode = %q, want '>> test'", Success("test"))
	}
	if Warn("msg") != "!! msg" {
		t.Errorf("Warn() in no-color mode = %q, want '!! msg'", Warn("msg"))
	}
}

func TestConsumeEscapeSequenceWaitsForFinalByte(t *testing.T) {
	ed := &lineEditor{line: []rune("abc"), cursor: 3, histPos: -1}
	if done := ed.consumeEscapeSequence([]byte{escapeChar, '['}); done {
		t.Fatal("incomplete escape sequence should keep collecting bytes")
	}
	if done := ed.consumeEscapeSequence([]byte{escapeChar, '[', 'D'}); !done {
		t.Fatal("complete left-arrow sequence should be consumed")
	}
	if ed.cursor != 2 {
		t.Fatalf("cursor = %d, want 2", ed.cursor)
	}
}

func TestBackspaceKeyEncodingsDeletePreviousRune(t *testing.T) {
	tests := []struct {
		name string
		key  byte
	}{
		{name: "DEL", key: backspaceKey},
		{name: "BS Ctrl-H", key: ctrlH},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !isBackspaceKey(tt.key) {
				t.Fatalf("key 0x%02x was not recognized as Backspace", tt.key)
			}
			ed := &lineEditor{line: []rune("a中b"), cursor: 2, histPos: -1}
			ed.backspace()
			if got := ed.string(); got != "ab" {
				t.Fatalf("line after Backspace = %q, want ab", got)
			}
			if ed.cursor != 1 {
				t.Fatalf("cursor after Backspace = %d, want 1", ed.cursor)
			}
		})
	}

	if isBackspaceKey('x') {
		t.Fatal("ordinary input was recognized as Backspace")
	}
}

func TestArrowCommandsNavigateHistory(t *testing.T) {
	ed := &lineEditor{
		history: []string{"list", "show prod"},
		histPos: -1,
		line:    []rune("draft"),
		cursor:  5,
	}
	ed.consumeEscapeSequence([]byte{escapeChar, '[', 'A'})
	if got := ed.string(); got != "show prod" {
		t.Fatalf("up arrow line = %q, want show prod", got)
	}
	ed.consumeEscapeSequence([]byte{escapeChar, '[', 'A'})
	if got := ed.string(); got != "list" {
		t.Fatalf("second up arrow line = %q, want list", got)
	}
	ed.consumeEscapeSequence([]byte{escapeChar, '[', 'B'})
	if got := ed.string(); got != "show prod" {
		t.Fatalf("down arrow line = %q, want show prod", got)
	}
}

func TestLineViewportKeepsLongInputWithinOneRow(t *testing.T) {
	line := []rune("0123456789abcdefghijklmnopqrstuvwxyz")
	visible, cursorColumn := lineViewport(line, len(line), 12)
	if got := visualLength(visible); got > 12 {
		t.Fatalf("visible width = %d, want <= 12: %q", got, visible)
	}
	if visible != "opqrstuvwxyz" {
		t.Fatalf("visible = %q, want tail of long input", visible)
	}
	if cursorColumn != 12 {
		t.Fatalf("cursor column = %d, want 12", cursorColumn)
	}
}

func TestLineViewportHandlesWideRunesAndMiddleCursor(t *testing.T) {
	line := []rune("ab你好cd")
	visible, cursorColumn := lineViewport(line, 4, 5)
	if visualLength(visible) > 5 {
		t.Fatalf("visible input wrapped: %q", visible)
	}
	if cursorColumn < 0 || cursorColumn > visualLength(visible) {
		t.Fatalf("cursor column %d is outside %q", cursorColumn, visible)
	}
}

func TestBracketedPasteBuffersAndNormalizesMultilineCommand(t *testing.T) {
	ed := &lineEditor{line: []rune("xt prod "), cursor: len([]rune("xt prod ")), histPos: -1}
	if done := ed.consumeEscapeSequence([]byte{escapeChar, '[', '2', '0', '0', '~'}); !done {
		t.Fatal("bracketed paste start should be consumed")
	}
	if !ed.pasting {
		t.Fatal("editor did not enter bracketed paste mode")
	}
	data := append([]byte("echo one\r\necho two\n"), bracketedPasteEnd...)
	completed := false
	for _, b := range data {
		completed = ed.consumePastedByte(b) || completed
	}
	if !completed || ed.pasting {
		t.Fatal("bracketed paste did not complete")
	}
	if got, want := ed.string(), "xt prod echo one echo two "; got != want {
		t.Fatalf("pasted line = %q, want %q", got, want)
	}
}

func TestNormalizePastedTextPreservesLongShellCommand(t *testing.T) {
	command := `for h in /sys/class/fc_host/host*; do printf "%s WWPN=%s\n" "$(basename "$h")" "$(cat "$h/port_name")"; done`
	if got := string(normalizePastedText([]byte(command))); got != command {
		t.Fatalf("normalized command = %q, want %q", got, command)
	}
}
