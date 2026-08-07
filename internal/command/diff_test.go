package command

import (
	"strings"
	"testing"
)

func TestUnifiedDiff(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
		want string
	}{
		{
			name: "identical",
			old:  "a\nb\nc\n",
			new:  "a\nb\nc\n",
			want: "",
		},
		{
			name: "insertion",
			old:  "a\nb\nc\n",
			new:  "a\nx\nb\nc\n",
			want: "@@ -1,3 +1,4 @@\n a\n+x\n b\n c\n",
		},
		{
			name: "deletion",
			old:  "a\nb\nc\n",
			new:  "a\nc\n",
			want: "@@ -1,3 +1,2 @@\n a\n-b\n c\n",
		},
		{
			name: "modification",
			old:  "a\nb\nc\n",
			new:  "a\nx\nc\n",
			want: "@@ -1,3 +1,3 @@\n a\n-b\n+x\n c\n",
		},
		{
			name: "insert at top",
			old:  "a\n",
			new:  "x\na\n",
			want: "@@ -1,1 +1,2 @@\n+x\n a\n",
		},
		{
			name: "delete at top",
			old:  "x\na\n",
			new:  "a\n",
			want: "@@ -1,2 +1,1 @@\n-x\n a\n",
		},
		{
			name: "insert into empty",
			old:  "",
			new:  "x\n",
			want: "@@ -0,0 +1,1 @@\n+x\n",
		},
		{
			name: "no trailing newline",
			old:  "a",
			new:  "b",
			want: "@@ -1,1 +1,1 @@\n-a\n+b\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unifiedDiff(tc.old, tc.new); got != tc.want {
				t.Fatalf("unifiedDiff =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestUnifiedDiffSplitsDistantHunks(t *testing.T) {
	var oldBuilder, newBuilder strings.Builder
	for i := 1; i <= 20; i++ {
		oldBuilder.WriteString("line ")
		oldBuilder.WriteString(itoa(i))
		oldBuilder.WriteByte('\n')
	}
	for i := 1; i <= 20; i++ {
		if i == 2 || i == 18 {
			newBuilder.WriteString("changed ")
			newBuilder.WriteString(itoa(i))
			newBuilder.WriteByte('\n')
			continue
		}
		newBuilder.WriteString("line ")
		newBuilder.WriteString(itoa(i))
		newBuilder.WriteByte('\n')
	}
	diff := unifiedDiff(oldBuilder.String(), newBuilder.String())
	if count := strings.Count(diff, "@@ -"); count != 2 {
		t.Fatalf("应拆成 2 个 hunk，实际 %d:\n%s", count, diff)
	}
	if !strings.Contains(diff, "changed 2") || !strings.Contains(diff, "changed 18") {
		t.Fatalf("diff 缺少变更行:\n%s", diff)
	}
}
