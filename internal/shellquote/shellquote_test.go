package shellquote

import "testing"

func TestSingle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"plain", "'plain'"},
		{"with space", "'with space'"},
		{"it's", `'it'"'"'s'`},
		{`a'b'c`, `'a'"'"'b'"'"'c'`},
		{"", "''"},
		{"$HOME", "'$HOME'"},
		{"`cmd`", "'`cmd`'"},
		{"$(x)", "'$(x)'"},
		{"; rm -rf /", "'; rm -rf /'"},
		{"新 行", "'新 行'"},
	}
	for _, tc := range cases {
		if got := Single(tc.in); got != tc.want {
			t.Errorf("Single(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCommand(t *testing.T) {
	got := Command([]string{"/path with space/ssh", "-o", "UserKnownHostsFile=/tmp/it's-here"})
	want := `'/path with space/ssh' '-o' 'UserKnownHostsFile=/tmp/it'"'"'s-here'`
	if got != want {
		t.Fatalf("Command = %q, want %q", got, want)
	}
}
