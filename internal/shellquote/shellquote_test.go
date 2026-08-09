package shellquote

import (
	"reflect"
	"testing"
)

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

func TestSplitAndCommandTreatShellGrammarAsLiteralData(t *testing.T) {
	args, err := Split(`printf '%s %s' "(id)" '$HOME;touch /tmp/pwn'`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"printf", "%s %s", "(id)", "$HOME;touch /tmp/pwn"}
	if len(args) != len(want) {
		t.Fatalf("Split() = %#v", args)
	}
	for index := range want {
		if args[index] != want[index] {
			t.Fatalf("Split()[%d] = %q, want %q", index, args[index], want[index])
		}
	}
	got := Command(args)
	if got != `'printf' '%s %s' '(id)' '$HOME;touch /tmp/pwn'` {
		t.Fatalf("safe command = %q", got)
	}
}

func TestSplitRejectsMalformedQuotesAndEscapes(t *testing.T) {
	for _, input := range []string{"echo 'unterminated", "echo trailing\\"} {
		if _, err := Split(input); err == nil {
			t.Fatalf("Split(%q) should fail", input)
		}
	}
}

func TestSplitPreservesOrdinaryBackslashInsideDoubleQuotes(t *testing.T) {
	args, err := Split(`grep "\\d+" file`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"grep", `\d+`, "file"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("Split() = %#v, want %#v", args, want)
	}
}
