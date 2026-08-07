package deploy

import "testing"

func TestRenderString(t *testing.T) {
	vars := Vars{"app_version": "1.4.2", "env": "prod", "count": 3}
	cases := []struct {
		template string
		want     string
	}{
		{"{{ app_version }}", "1.4.2"},
		{"v{{ app_version }}-{{ env }}", "v1.4.2-prod"},
		{"{{ app_version | upper }}", "1.4.2"},
		{"{{ env | upper }}", "PROD"},
		{"{{ var \"missing\" | default \"fallback\" }}", "fallback"},
		{"{{ port | default 8080 }}", "8080"},
		{"{{ missing | default \"x\" | upper }}", "X"},
		{"count={{ count }}", "count=3"},
		{"no template", "no template"},
	}
	for _, tc := range cases {
		got, err := RenderString(tc.template, vars)
		if err != nil {
			t.Fatalf("RenderString(%q) error: %v", tc.template, err)
		}
		if got != tc.want {
			t.Errorf("RenderString(%q) = %q, want %q", tc.template, got, tc.want)
		}
	}
}

func TestRenderStringMissingVarFails(t *testing.T) {
	if _, err := RenderString("{{ missing }}", Vars{}); err == nil {
		t.Fatal("缺失变量应当报错")
	}
}

func TestRenderStringFilters(t *testing.T) {
	vars := Vars{
		"envs": []any{"dev", "prod"},
		"name": "  web01  ",
		"text": "aXb",
		"port": 0,
	}
	cases := []struct {
		template string
		want     string
	}{
		{"{{ envs | join \",\" }}", "dev,prod"},
		{"{{ text | replace \"X\" \"-\" }}", "a-b"},
		{"{{ name | trim }}", "web01"},
		{"{{ name | trim | upper }}", "WEB01"},
		{"{{ name | shellquote }}", `'  web01  '`},
		{"{{ port | default 8080 }}", "0"},
		{"x {{- name -}} y", "x  web01  y"},
	}
	for _, tc := range cases {
		got, err := RenderString(tc.template, vars)
		if err != nil {
			t.Fatalf("RenderString(%q) error: %v", tc.template, err)
		}
		if got != tc.want {
			t.Errorf("RenderString(%q) = %q, want %q", tc.template, got, tc.want)
		}
	}
}

func TestRenderStringNestedMissingFails(t *testing.T) {
	if _, err := RenderString("{{ a.b }}", Vars{"a": Vars{}}); err == nil {
		t.Fatal("嵌套缺失变量应当报错")
	}
}

func TestRenderStringUnknownFunctionFails(t *testing.T) {
	if _, err := RenderString("{{ nope \"x\" }}", Vars{}); err == nil {
		t.Fatal("未知模板函数应当报错")
	}
}
