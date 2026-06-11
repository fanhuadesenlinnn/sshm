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
		{"你好", 2},   // visualLength counts runes, not display width
		{"a你好b", 4}, // 4 runes
		{"日本語", 3},  // 3 runes
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
		{"a", 1, ""},
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
