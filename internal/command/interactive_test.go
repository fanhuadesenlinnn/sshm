package command

import (
	"reflect"
	"testing"

	"github.com/sshm/sshm/internal/config"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"hello", []string{"hello"}},
		{"hello world", []string{"hello", "world"}},
		{"cmd arg1 arg2", []string{"cmd", "arg1", "arg2"}},
		{`cmd "arg with spaces"`, []string{"cmd", "arg with spaces"}},
		{`cmd 'single quoted'`, []string{"cmd", "single quoted"}},
		{"  leading space", []string{"leading", "space"}},
		{"trailing space  ", []string{"trailing", "space"}},
		{"multiple   spaces", []string{"multiple", "spaces"}},
	}

	for _, tt := range tests {
		got := parseArgs(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseArgs(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestMatchHost(t *testing.T) {
	h := config.Host{
		Alias: "webserver",
		Host:  "10.0.0.1",
		User:  "admin",
		Note:  "production web server",
		Group: "prod",
		Tags:  []string{"web", "nginx"},
	}

	tests := []struct {
		keyword string
		want    bool
	}{
		{"web", true},        // matches alias
		{"server", true},     // matches alias
		{"10.0", true},       // matches host
		{"admin", true},      // matches user
		{"production", true}, // matches note
		{"prod", true},       // matches group
		{"nginx", true},      // matches tag
		{"nonexistent", false},
		{"database", false},
		{"", true}, // empty keyword matches everything (contains check)
	}

	for _, tt := range tests {
		got := matchHost(h, tt.keyword)
		if got != tt.want {
			t.Errorf("matchHost(host, %q) = %v, want %v", tt.keyword, got, tt.want)
		}
	}
}
