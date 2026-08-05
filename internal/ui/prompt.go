package ui

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

const (
	maxHistory   = 100
	escapeChar   = 27
	backspaceKey = 127
	ctrlH        = 8
	ctrlC        = 3
	ctrlD        = 4
	enterKey     = 13
	newlineKey   = 10
)

var bracketedPasteEnd = []byte{escapeChar, '[', '2', '0', '1', '~'}

// lineEditor holds the state for a single interactive readline session.
type lineEditor struct {
	history   []string
	histPos   int    // -1 = new line; 0..len(history)-1 = browsing
	histDirty string // the in-progress line when user starts browsing history

	line   []rune
	cursor int // cursor position in runes
	prompt string

	pasting    bool
	pasteBytes []byte
}

// global history shared across all ReadLine calls.
var globalHistory []string

// ReadLine reads a line from stdin with full line-editing support.
// Supports: ← → cursor movement, ↑ ↓ history, Home/End, Backspace/Delete.
func ReadLine(prompt string) string {
	if !isTerminal() {
		return readLineFallback(prompt)
	}

	ed := &lineEditor{
		history: globalHistory,
		histPos: -1,
		prompt:  prompt,
	}

	return ed.run()
}

// ReadLineDefault reads a line with a default value pre-filled.
func ReadLineDefault(prompt string, defaultVal string) string {
	if !isTerminal() {
		return readLineFallbackDefault(prompt, defaultVal)
	}

	ed := &lineEditor{
		history:   globalHistory,
		histPos:   -1,
		prompt:    prompt,
		line:      []rune(defaultVal),
		cursor:    len([]rune(defaultVal)),
		histDirty: defaultVal,
	}

	return ed.run()
}

// ReadYesNo reads a yes/no answer (default no).
func ReadYesNo(prompt string) bool {
	input := ReadLine(prompt)
	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes"
}

// ReadPassword reads a password without echoing.
func ReadPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	pass, err := readPasswordFromTTY()
	if err != nil {
		return "", err
	}
	fmt.Fprintln(os.Stderr)
	return pass, nil
}

// IsTerminal returns true if stdin is a terminal.
func IsTerminal() bool {
	return isTerminal()
}

// run is the main event loop for the line editor.
func (ed *lineEditor) run() string {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Cannot enter raw mode; fall back to simple read
		return readLineFallback(ed.prompt)
	}
	defer term.Restore(fd, oldState)
	fmt.Fprint(os.Stderr, "\033[?2004h")
	defer fmt.Fprint(os.Stderr, "\033[?2004l")

	// Write initial prompt and line content
	ed.redraw()

	var escSeq [8]byte
	escLen := 0
	var runeBytes []byte

	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			ed.finishLine()
			return ed.string()
		}

		b := buf[0]
		if ed.pasting {
			if ed.consumePastedByte(b) {
				ed.redraw()
			}
			continue
		}

		// If we're in the middle of an escape sequence
		if escLen > 0 {
			escSeq[escLen] = b
			escLen++

			// Keep collecting incomplete sequences such as ESC [ until the final key byte arrives.
			if ed.consumeEscapeSequence(escSeq[:escLen]) {
				escLen = 0
			} else if escLen >= 8 {
				// Too long, give up
				escLen = 0
			}
			continue
		}

		if isBackspaceKey(b) {
			ed.backspace()
			ed.redraw()
			continue
		}

		switch b {
		case escapeChar:
			escSeq[0] = b
			escLen = 1

		case ctrlC:
			ed.finishLine()
			return ""

		case ctrlD:
			if len(ed.line) == 0 {
				ed.finishLine()
				return ""
			}
			ed.deleteForward()
			ed.redraw()

		case enterKey, newlineKey:
			ed.finishLine()
			result := ed.string()
			if result != "" {
				addToHistory(result)
			}
			return result

		default:
			if b >= 32 && b < 127 {
				runeBytes = runeBytes[:0]
				ed.insert(rune(b))
				ed.redraw()
			} else if b >= utf8.RuneSelf {
				runeBytes = append(runeBytes, b)
				if utf8.FullRune(runeBytes) {
					r, size := utf8.DecodeRune(runeBytes)
					if r != utf8.RuneError || size > 1 {
						ed.insert(r)
						ed.redraw()
					}
					runeBytes = runeBytes[:0]
				}
			}
		}
	}
}

// isBackspaceKey accepts both terminal encodings commonly produced by the
// Backspace key: DEL (0x7f) and BS/Ctrl-H (0x08).
func isBackspaceKey(b byte) bool {
	return b == backspaceKey || b == ctrlH
}

// consumeEscapeSequence handles a complete escape sequence and reports whether
// the caller should stop collecting bytes.
func (ed *lineEditor) consumeEscapeSequence(seq []byte) bool {
	cmd := ed.parseEscape(seq)
	if cmd == escCmdIncomplete {
		return false
	}
	if cmd != escCmdUnknown {
		ed.handleEscapeCmd(cmd, seq)
	}
	return true
}

// escape command constants
const (
	escCmdUnknown    = -1
	escCmdIncomplete = 0
	escCmdUp         = 1
	escCmdDown       = 2
	escCmdRight      = 3
	escCmdLeft       = 4
	escCmdHome       = 5
	escCmdEnd        = 6
	escCmdDelete     = 7
	escCmdPasteStart = 8
	escCmdPasteEnd   = 9
)

// parseEscape tries to parse an ANSI escape sequence.
// Returns escCmdIncomplete if more bytes are needed, escCmdUnknown if unrecognized.
func (ed *lineEditor) parseEscape(seq []byte) int {
	if len(seq) == 0 {
		return escCmdUnknown
	}

	if seq[0] != escapeChar {
		return escCmdUnknown
	}

	if len(seq) == 1 {
		return escCmdIncomplete
	}

	switch seq[1] {
	case '[':
		// CSI sequences: ESC [ ...
		if len(seq) == 2 {
			return escCmdIncomplete
		}
		switch seq[2] {
		case 'A':
			return escCmdUp
		case 'B':
			return escCmdDown
		case 'C':
			return escCmdRight
		case 'D':
			return escCmdLeft
		case 'H':
			return escCmdHome
		case 'F':
			return escCmdEnd
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			// Could be Home/End/Delete with tilde: ESC [ 1 ~ etc.
			if len(seq) == 3 {
				return escCmdIncomplete
			}
			for i := 2; i < len(seq); i++ {
				if seq[i] == '~' {
					if i != len(seq)-1 {
						return escCmdUnknown
					}
					switch string(seq[2:i]) {
					case "1", "7":
						return escCmdHome
					case "4", "8":
						return escCmdEnd
					case "3":
						return escCmdDelete
					case "200":
						return escCmdPasteStart
					case "201":
						return escCmdPasteEnd
					default:
						return escCmdUnknown
					}
				}
				if seq[i] < '0' || seq[i] > '9' {
					return escCmdUnknown
				}
			}
			return escCmdIncomplete
		default:
			return escCmdUnknown
		}

	case 'O':
		// SS3 sequences: ESC O X (xterm alternate mode)
		if len(seq) == 2 {
			return escCmdIncomplete
		}
		switch seq[2] {
		case 'A':
			return escCmdUp
		case 'B':
			return escCmdDown
		case 'C':
			return escCmdRight
		case 'D':
			return escCmdLeft
		case 'H':
			return escCmdHome
		case 'F':
			return escCmdEnd
		default:
			return escCmdUnknown
		}

	default:
		// ALT+key produces ESC followed by a regular character.
		// Treat as ESC then insert the character.
		return escCmdUnknown
	}
}

// handleEscapeCmd executes an escape command.
func (ed *lineEditor) handleEscapeCmd(cmd int, _ []byte) {
	switch cmd {
	case escCmdUp:
		ed.historyUp()
		ed.redraw()
	case escCmdDown:
		ed.historyDown()
		ed.redraw()
	case escCmdRight:
		ed.cursorRight()
		ed.redraw()
	case escCmdLeft:
		ed.cursorLeft()
		ed.redraw()
	case escCmdHome:
		ed.cursor = 0
		ed.redraw()
	case escCmdEnd:
		ed.cursor = len(ed.line)
		ed.redraw()
	case escCmdDelete:
		ed.deleteForward()
		ed.redraw()
	case escCmdPasteStart:
		ed.pasting = true
		ed.pasteBytes = ed.pasteBytes[:0]
	}
}

// consumePastedByte buffers bracketed paste input until the terminal's end
// marker arrives. The caller redraws once after this method returns true.
func (ed *lineEditor) consumePastedByte(b byte) bool {
	ed.pasteBytes = append(ed.pasteBytes, b)
	if !bytes.HasSuffix(ed.pasteBytes, bracketedPasteEnd) {
		return false
	}
	payload := ed.pasteBytes[:len(ed.pasteBytes)-len(bracketedPasteEnd)]
	ed.insertRunes(normalizePastedText(payload))
	ed.pasteBytes = ed.pasteBytes[:0]
	ed.pasting = false
	return true
}

func normalizePastedText(data []byte) []rune {
	text := strings.ToValidUTF8(string(data), "")
	result := make([]rune, 0, len(text))
	pendingSpace := false
	for _, r := range text {
		switch r {
		case '\r', '\n', '\t':
			pendingSpace = true
			continue
		}
		if r < 32 || r == 127 {
			continue
		}
		if pendingSpace && len(result) > 0 && result[len(result)-1] != ' ' {
			result = append(result, ' ')
		}
		pendingSpace = false
		result = append(result, r)
	}
	if pendingSpace && len(result) > 0 && result[len(result)-1] != ' ' {
		result = append(result, ' ')
	}
	return result
}

// insert inserts a rune at the current cursor position.
func (ed *lineEditor) insert(r rune) {
	line := ed.line
	if ed.cursor == len(line) {
		ed.line = append(ed.line, r)
	} else {
		// Insert in middle: make room
		ed.line = append(line[:ed.cursor], append([]rune{r}, line[ed.cursor:]...)...)
	}
	ed.cursor++
}

func (ed *lineEditor) insertRunes(value []rune) {
	if len(value) == 0 {
		return
	}
	line := make([]rune, 0, len(ed.line)+len(value))
	line = append(line, ed.line[:ed.cursor]...)
	line = append(line, value...)
	line = append(line, ed.line[ed.cursor:]...)
	ed.line = line
	ed.cursor += len(value)
}

// backspace deletes the character before the cursor.
func (ed *lineEditor) backspace() {
	if ed.cursor > 0 {
		ed.line = append(ed.line[:ed.cursor-1], ed.line[ed.cursor:]...)
		ed.cursor--
	}
}

// deleteForward deletes the character at the cursor.
func (ed *lineEditor) deleteForward() {
	if ed.cursor < len(ed.line) {
		ed.line = append(ed.line[:ed.cursor], ed.line[ed.cursor+1:]...)
	}
}

// cursorLeft moves cursor one position left.
func (ed *lineEditor) cursorLeft() {
	if ed.cursor > 0 {
		ed.cursor--
	}
}

// cursorRight moves cursor one position right.
func (ed *lineEditor) cursorRight() {
	if ed.cursor < len(ed.line) {
		ed.cursor++
	}
}

// historyUp navigates to the previous history entry.
func (ed *lineEditor) historyUp() {
	if len(ed.history) == 0 {
		return
	}
	if ed.histPos == -1 {
		ed.histDirty = string(ed.line)
		ed.histPos = len(ed.history) - 1
	} else if ed.histPos > 0 {
		ed.histPos--
	}
	ed.line = []rune(ed.history[ed.histPos])
	ed.cursor = len(ed.line)
}

// historyDown navigates to the next history entry.
func (ed *lineEditor) historyDown() {
	if ed.histPos == -1 {
		return
	}
	if ed.histPos < len(ed.history)-1 {
		ed.histPos++
		ed.line = []rune(ed.history[ed.histPos])
	} else {
		ed.histPos = -1
		ed.line = []rune(ed.histDirty)
		ed.histDirty = ""
	}
	ed.cursor = len(ed.line)
}

// redraw clears the current line and redraws it.
func (ed *lineEditor) redraw() {
	width := promptTerminalWidth()
	available := width - visualLength(ed.prompt) - 1
	if available < 1 {
		available = 1
	}
	visible, cursorColumn := lineViewport(ed.line, ed.cursor, available)
	fmt.Fprintf(os.Stderr, "\r\033[2K%s%s", ed.prompt, visible)
	if tailWidth := visualLength(visible) - cursorColumn; tailWidth > 0 {
		fmt.Fprintf(os.Stderr, "\033[%dD", tailWidth)
	}
}

// lineViewport returns a single-row window containing the cursor. Keeping the
// rendered input narrower than the terminal prevents automatic wrapping from
// leaving stale copies of long pasted commands on screen.
func lineViewport(line []rune, cursor, maxWidth int) (string, int) {
	if maxWidth < 1 {
		return "", 0
	}
	cursor = max(0, min(cursor, len(line)))
	start := 0
	cursorColumn := runesDisplayWidth(line[:cursor])
	for start < cursor && cursorColumn > maxWidth {
		cursorColumn -= runeDisplayWidth(line[start])
		start++
	}
	end := start
	width := 0
	for end < len(line) {
		runeWidth := runeDisplayWidth(line[end])
		if width+runeWidth > maxWidth {
			break
		}
		width += runeWidth
		end++
	}
	return string(line[start:end]), cursorColumn
}

func runesDisplayWidth(value []rune) int {
	width := 0
	for _, r := range value {
		width += runeDisplayWidth(r)
	}
	return width
}

func promptTerminalWidth() int {
	if width, _, err := term.GetSize(int(os.Stdin.Fd())); err == nil && width > 0 {
		return width
	}
	return 80
}

// finishLine prints a newline to finalize the input.
func (ed *lineEditor) finishLine() {
	fmt.Fprint(os.Stderr, "\r\n")
}

// string returns the current line content.
func (ed *lineEditor) string() string {
	if ed.line == nil {
		return ""
	}
	return string(ed.line)
}

// addToHistory adds a line to the global history.
func addToHistory(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if len(globalHistory) > 0 && globalHistory[len(globalHistory)-1] == line {
		return
	}
	globalHistory = append(globalHistory, line)
	if len(globalHistory) > maxHistory {
		globalHistory = globalHistory[len(globalHistory)-maxHistory:]
	}
}

// visualLength estimates the terminal column width of a string.
func visualLength(s string) int {
	return displayWidth(s)
}

// readLineFallback reads a line without raw mode (for piped/redirected input).
func readLineFallback(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	var buf strings.Builder
	var b [1]byte
	for {
		n, err := os.Stdin.Read(b[:])
		if err != nil || n == 0 {
			break
		}
		if b[0] == '\n' {
			break
		}
		if b[0] != '\r' {
			buf.WriteByte(b[0])
		}
	}
	return strings.TrimRight(buf.String(), "\r\n")
}

// readLineFallbackDefault reads with a default value without raw mode.
func readLineFallbackDefault(prompt string, defaultVal string) string {
	line := readLineFallback(prompt)
	if line == "" {
		return defaultVal
	}
	return line
}
