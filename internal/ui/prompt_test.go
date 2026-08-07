package ui

import (
	"testing"
)

func newEditor(line string) *lineEditor {
	runes := []rune(line)
	return &lineEditor{line: runes, cursor: len(runes), histPos: -1}
}

func TestLineEditorInsertAndDelete(t *testing.T) {
	ed := newEditor("hel")
	ed.insert('l')
	ed.insert('o')
	if got := ed.string(); got != "hello" {
		t.Fatalf("insert = %q", got)
	}

	ed = newEditor("hello")
	ed.cursor = 2
	ed.insert('X')
	if got := ed.string(); got != "heXllo" {
		t.Fatalf("middle insert = %q", got)
	}

	ed = newEditor("hello")
	ed.cursor = 3
	ed.backspace()
	if got := ed.string(); got != "helo" || ed.cursor != 2 {
		t.Fatalf("backspace = %q cursor=%d", got, ed.cursor)
	}
	ed.cursor = 0
	ed.backspace() // 边界不应越界
	if got := ed.string(); got != "helo" || ed.cursor != 0 {
		t.Fatalf("backspace 边界 = %q cursor=%d", got, ed.cursor)
	}

	ed = newEditor("hello")
	ed.cursor = 2
	ed.deleteForward()
	if got := ed.string(); got != "helo" {
		t.Fatalf("deleteForward = %q", got)
	}
	ed.cursor = len(ed.line)
	ed.deleteForward() // 边界不应越界
	if got := ed.string(); got != "helo" {
		t.Fatalf("deleteForward 边界 = %q", got)
	}
}

func TestLineEditorUnicode(t *testing.T) {
	ed := newEditor("你好")
	ed.insert('世')
	if got := ed.string(); got != "你好世" || ed.cursor != 3 {
		t.Fatalf("unicode insert = %q cursor=%d", got, ed.cursor)
	}
	ed.cursor = 1
	ed.backspace()
	if got := ed.string(); got != "好世" {
		t.Fatalf("unicode backspace = %q", got)
	}
}

func TestLineEditorCursorMovement(t *testing.T) {
	ed := newEditor("abc")
	ed.cursorLeft()
	ed.cursorLeft()
	if ed.cursor != 1 {
		t.Fatalf("cursor = %d", ed.cursor)
	}
	ed.cursorLeft()
	ed.cursorLeft() // 左边界
	if ed.cursor != 0 {
		t.Fatalf("cursor 左边界 = %d", ed.cursor)
	}
	ed.cursorRight()
	ed.cursorRight()
	ed.cursorRight()
	ed.cursorRight() // 右边界
	if ed.cursor != 3 {
		t.Fatalf("cursor 右边界 = %d", ed.cursor)
	}
}

func TestLineEditorHistory(t *testing.T) {
	ed := &lineEditor{history: []string{"first", "second"}, histPos: -1, line: []rune("dirty"), cursor: 5}
	ed.historyUp()
	if got := ed.string(); got != "second" || ed.histDirty != "dirty" {
		t.Fatalf("historyUp = %q dirty=%q", got, ed.histDirty)
	}
	ed.historyUp()
	if got := ed.string(); got != "first" {
		t.Fatalf("historyUp2 = %q", got)
	}
	ed.historyDown()
	if got := ed.string(); got != "second" {
		t.Fatalf("historyDown = %q", got)
	}
	ed.historyDown()
	if got := ed.string(); got != "dirty" || ed.histPos != -1 {
		t.Fatalf("historyDown 恢复 = %q pos=%d", got, ed.histPos)
	}
	ed.historyDown() // 空历史/未进入浏览不应变化
	if got := ed.string(); got != "dirty" {
		t.Fatalf("historyDown 空状态 = %q", got)
	}
}

func TestParseEscapeSequences(t *testing.T) {
	cases := []struct {
		seq  string
		want int
	}{
		{"\x1b[A", escCmdUp},
		{"\x1b[B", escCmdDown},
		{"\x1b[C", escCmdRight},
		{"\x1b[D", escCmdLeft},
		{"\x1b[H", escCmdHome},
		{"\x1b[F", escCmdEnd},
		{"\x1b[1~", escCmdHome},
		{"\x1b[7~", escCmdHome},
		{"\x1b[4~", escCmdEnd},
		{"\x1b[8~", escCmdEnd},
		{"\x1b[3~", escCmdDelete},
		{"\x1b[200~", escCmdPasteStart},
		{"\x1b[201~", escCmdPasteEnd},
		{"\x1bOA", escCmdUp},
		{"\x1bOD", escCmdLeft},
		{"\x1b", escCmdIncomplete},
		{"\x1b[", escCmdIncomplete},
		{"\x1b[1", escCmdIncomplete},
		{"\x1b[Z", escCmdUnknown},
		{"abc", escCmdUnknown},
	}
	for _, tc := range cases {
		if got := (&lineEditor{}).parseEscape([]byte(tc.seq)); got != tc.want {
			t.Errorf("parseEscape(%q) = %d, want %d", tc.seq, got, tc.want)
		}
	}
}

func TestConsumeEscapeSequenceAppliesCommand(t *testing.T) {
	ed := newEditor("abc")
	ed.cursor = 1
	if !ed.consumeEscapeSequence([]byte("\x1b[C")) {
		t.Fatal("完整序列应结束收集")
	}
	if ed.cursor != 2 {
		t.Fatalf("右移后 cursor = %d", ed.cursor)
	}
	ed.history = []string{"old"}
	ed.line = []rune("new")
	ed.cursor = 3
	if !ed.consumeEscapeSequence([]byte("\x1b[A")) {
		t.Fatal("完整序列应结束收集")
	}
	if got := ed.string(); got != "old" {
		t.Fatalf("上箭头应进入历史: %q", got)
	}
	if ed.consumeEscapeSequence([]byte("\x1b")) {
		t.Fatal("不完整序列应继续收集")
	}
}

func TestNormalizePastedText(t *testing.T) {
	got := string(normalizePastedText([]byte("line1\nline2\ttab\r\nend\x00\x01")))
	if got != "line1 line2 tab end" {
		t.Fatalf("normalizePastedText = %q", got)
	}
	if trailing := string(normalizePastedText([]byte("a\n"))); trailing != "a" {
		t.Fatalf("尾部换行不应补空格: %q", trailing)
	}
}

func TestConsumePastedByte(t *testing.T) {
	ed := newEditor("")
	ed.pasting = true
	payload := []byte("pasted text")
	for _, b := range payload {
		if ed.consumePastedByte(b) {
			t.Fatal("结束标记前不应完成粘贴")
		}
	}
	for _, b := range bracketedPasteEnd {
		ed.consumePastedByte(b)
	}
	if ed.pasting || ed.string() != "pasted text" {
		t.Fatalf("粘贴结果 = %q pasting=%t", ed.string(), ed.pasting)
	}
}

func TestLineViewportKeepsCursorVisible(t *testing.T) {
	line := []rune("abcdef")
	visible, cursorColumn := lineViewport(line, 6, 3)
	if visible != "def" || cursorColumn != 3 {
		t.Fatalf("viewport = %q column=%d", visible, cursorColumn)
	}
	visible, cursorColumn = lineViewport(line, 2, 10)
	if visible != "abcdef" || cursorColumn != 2 {
		t.Fatalf("viewport 全宽 = %q column=%d", visible, cursorColumn)
	}
}

func TestAddToHistory(t *testing.T) {
	globalHistory = nil
	addToHistory("  first  ")
	addToHistory("second")
	addToHistory("second") // 去重
	addToHistory("")
	if len(globalHistory) != 2 || globalHistory[0] != "first" || globalHistory[1] != "second" {
		t.Fatalf("history = %v", globalHistory)
	}
	globalHistory = make([]string, maxHistory)
	addToHistory("overflow")
	if len(globalHistory) != maxHistory || globalHistory[maxHistory-1] != "overflow" {
		t.Fatalf("history 上限 = %d", len(globalHistory))
	}
}

func TestCommitLineHistoryBehavior(t *testing.T) {
	globalHistory = nil
	ed := newEditor("hello")
	if got := ed.commitLine(); got != "hello" {
		t.Fatalf("commitLine = %q", got)
	}
	if len(globalHistory) != 1 || globalHistory[0] != "hello" {
		t.Fatalf("普通输入应进历史: %v", globalHistory)
	}
	ed = newEditor("yes")
	ed.noHistory = true
	if got := ed.commitLine(); got != "yes" {
		t.Fatalf("noHistory commitLine = %q", got)
	}
	if len(globalHistory) != 1 {
		t.Fatalf("noHistory 不应污染历史: %v", globalHistory)
	}
}

func TestIsBackspaceKey(t *testing.T) {
	if !isBackspaceKey(backspaceKey) || !isBackspaceKey(ctrlH) || isBackspaceKey('x') {
		t.Fatal("isBackspaceKey 判定错误")
	}
}

func TestRunesDisplayWidth(t *testing.T) {
	if got := runesDisplayWidth([]rune("a中")); got != 3 {
		t.Fatalf("runesDisplayWidth = %d", got)
	}
	if got := displayWidth("\x1b[31mred\x1b[0m"); got != 3 {
		t.Fatalf("displayWidth(ANSI) = %d", got)
	}
}
