package deploy

import "testing"

func TestEvalWhen(t *testing.T) {
	env := map[string]any{
		"app_version": "1.4.2",
		"upload":      map[string]any{"changed": true, "rc": 0},
		"count":       int64(3),
		"name":        "web01",
		"envs":        []any{"dev", "prod"},
		"groups":      []string{"web", "db"},
		"empty_list":  []any{},
		"empty_map":   map[string]any{},
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
		{"not upload.changed", false},
		{"count > 2", true},
		{"count <= 3", true},
		{"count >= 3", true},
		{"name == 'web01' || name == 'web02'", true},
		{"(count > 1) && (name == 'web01')", true},
		{"app_version == '1.4.2' && (count > 10 || name == 'web01')", true},
		{"-1 < count", true},
		{"count == 3", true},
		{"count == 3.0", true},
		{"'prod' in envs", true},
		{"'staging' in envs", false},
		{"'web' in groups", true},
		{"'web' not in groups", false},
		{"'staging' not in envs", true},
		{"'web01' in name", true},
		{"'db01' in name", false},
		{"upload is defined", true},
		{"missing is defined", false},
		{"missing is not defined", true},
		{"missing.changed is defined", false},
		{"upload.changed is defined", true},
		{"'value' is defined", true},
		{"missing is defined and missing.changed", false},
		{"missing is not defined or upload.changed", true},
		{"empty_list", false},
		{"empty_map", false},
		{"envs", true},
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
	for _, expression := range []string{"upload.changed &&", "== 1", "'unterminated", "a b", "foo is", "foo is wat", "foo not wat"} {
		if _, err := EvalWhen(expression, map[string]any{}); err == nil {
			t.Errorf("EvalWhen(%q) 应当报错", expression)
		}
	}
}

func TestEvalWhenUndefinedVariableErrors(t *testing.T) {
	for _, expression := range []string{
		"missing == 'x'",
		"missing.changed",
		"missing != 'x'",
		"missing < 3",
		"not missing",
	} {
		if _, err := EvalWhen(expression, map[string]any{}); err == nil {
			t.Errorf("EvalWhen(%q) 未定义变量应当报错", expression)
		}
	}
}

func TestEvalWhenShortCircuitSkipsUndefined(t *testing.T) {
	env := map[string]any{"upload": map[string]any{"changed": true}}
	got, err := EvalWhen("missing is defined and missing.changed", env)
	if err != nil {
		t.Fatalf("短路求值不应触发未定义错误: %v", err)
	}
	if got {
		t.Fatalf("missing is defined 应为 false")
	}
	got, err = EvalWhen("true or missing.changed", env)
	if err != nil {
		t.Fatalf("|| 短路求值不应触发未定义错误: %v", err)
	}
	if !got {
		t.Fatalf("true or ... 应为 true")
	}
}

func TestEvalWhenStructuredComparison(t *testing.T) {
	env := map[string]any{
		"a":     []any{"x", "y"},
		"b":     []any{"x", "y"},
		"c":     []any{"x"},
		"map_a": map[string]any{"k": "v"},
		"map_b": map[string]any{"k": "v"},
	}
	cases := []struct {
		expression string
		want       bool
	}{
		{"a == b", true},
		{"a != c", true},
		{"a == c", false},
		{"map_a == map_b", true},
		{"map_a != map_b", false},
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

func TestEvalWhenOrderingStructuredValuesFails(t *testing.T) {
	env := map[string]any{
		"a": []any{"x"},
		"b": []any{"x"},
	}
	for _, expression := range []string{"a < b", "a >= b", "1 < 'x'"} {
		if _, err := EvalWhen(expression, env); err == nil {
			t.Errorf("EvalWhen(%q) 不可排序比较应当报错", expression)
		}
	}
}

func TestEvalWhenMembershipTypeError(t *testing.T) {
	env := map[string]any{"count": 3, "a": []any{"x"}}
	if _, err := EvalWhen("'x' in count", env); err == nil {
		t.Fatal("in 右侧不是列表/字符串应当报错")
	}
	if _, err := EvalWhen("1 in missing", env); err == nil {
		t.Fatal("in 右侧未定义应当报错")
	}
	if _, err := EvalWhen("1 in a", env); err != nil {
		t.Fatalf("列表成员比较不应报错: %v", err)
	}
}

func TestParseWhenValidatesSyntaxOnly(t *testing.T) {
	if err := ParseWhen("upload.changed"); err != nil {
		t.Fatalf("ParseWhen 不应因运行时变量缺失而失败: %v", err)
	}
	if err := ParseWhen("missing == 'x'"); err != nil {
		t.Fatalf("ParseWhen 只检查语法，不应因未定义变量失败: %v", err)
	}
	if err := ParseWhen("upload.changed &&"); err == nil {
		t.Fatal("ParseWhen 应拒绝残缺表达式")
	}
}
