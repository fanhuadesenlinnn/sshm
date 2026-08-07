package deployv3

import "testing"

func TestEvalWhen(t *testing.T) {
	env := map[string]any{
		"app_version": "1.4.2",
		"upload":      map[string]any{"changed": true, "rc": 0},
		"count":       int64(3),
		"name":        "web01",
	}
	cases := []struct {
		expression string
		want       bool
	}{
		{"app_version == '1.4.2'", true},
		{"app_version != '1.4.3'", true},
		{"upload.changed", true},
		{"upload.changed && upload.rc == 0", true},
		{"upload.changed && upload.rc != 0", false},
		{"!upload.changed", false},
		{"count > 2", true},
		{"count <= 3", true},
		{"name == 'web01' || name == 'web02'", true},
		{"missing == 'x'", false},
		{"missing.changed", false},
		{"(count > 1) && (name == 'web01')", true},
		{"app_version == '1.4.2' && (count > 10 || name == 'web01')", true},
	}
	for _, tc := range cases {
		got, err := EvalWhen(tc.expression, env)
		if err != nil {
			t.Fatalf("EvalWhen(%q) error: %v", tc.expression, err)
		}
		if got != tc.want {
			t.Errorf("EvalWhen(%q) = %v, want %v", tc.expression, got, tc.want)
		}
	}
}

func TestEvalWhenRejectsBadSyntax(t *testing.T) {
	for _, expression := range []string{"upload.changed &&", "== 1", "'unterminated", "a b"} {
		if _, err := EvalWhen(expression, map[string]any{}); err == nil {
			t.Errorf("EvalWhen(%q) 应当报错", expression)
		}
	}
}
