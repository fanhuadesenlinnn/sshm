package config

import (
	"testing"
	"time"
)

func TestParseHumanDuration(t *testing.T) {
	tests := map[string]time.Duration{
		"30s": 30 * time.Second,
		"5m":  5 * time.Minute,
		"1h":  time.Hour,
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
		"90d": 90 * 24 * time.Hour,
	}
	for value, want := range tests {
		got, err := ParseHumanDuration(value)
		if err != nil {
			t.Fatalf("ParseHumanDuration(%q): %v", value, err)
		}
		if got != want {
			t.Fatalf("ParseHumanDuration(%q) = %s, want %s", value, got, want)
		}
	}
	for _, value := range []string{"", "0s", "-1s", "tomorrow", "d"} {
		if _, err := ParseHumanDuration(value); err == nil {
			t.Fatalf("ParseHumanDuration(%q) should fail", value)
		}
	}
}
