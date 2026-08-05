package ui

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"golang.org/x/term"
)

// PickHost opens a lightweight live-filtering host picker.
func PickHost(hosts []config.Host) (string, bool) {
	if len(hosts) == 0 || !isTerminal() {
		return "", false
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", false
	}
	defer term.Restore(fd, oldState)
	fmt.Fprint(os.Stderr, "\033[?25l")
	defer fmt.Fprint(os.Stderr, "\033[?25h")

	query := ""
	selected := 0
	var runeBytes []byte
	for {
		matches := filterPickerHosts(hosts, query)
		if selected >= len(matches) {
			selected = max(0, len(matches)-1)
		}
		renderHostPicker(matches, query, selected)

		var input [1]byte
		if _, err := os.Stdin.Read(input[:]); err != nil {
			return "", false
		}
		if isBackspaceKey(input[0]) {
			runes := []rune(query)
			if len(runes) > 0 {
				query = string(runes[:len(runes)-1])
				selected = 0
			}
			continue
		}
		switch input[0] {
		case ctrlC, ctrlD:
			clearPicker()
			return "", false
		case enterKey, newlineKey:
			if len(matches) == 0 {
				continue
			}
			clearPicker()
			return matches[selected].Alias, true
		case escapeChar:
			var sequence [2]byte
			if _, err := io.ReadFull(os.Stdin, sequence[:]); err != nil || sequence[0] != '[' {
				continue
			}
			switch sequence[1] {
			case 'A':
				if selected > 0 {
					selected--
				}
			case 'B':
				if selected+1 < len(matches) {
					selected++
				}
			}
		default:
			if input[0] >= 32 && input[0] < utf8.RuneSelf {
				if input[0] == 'q' && query == "" {
					clearPicker()
					return "", false
				}
				query += string(rune(input[0]))
				selected = 0
			} else if input[0] >= utf8.RuneSelf {
				runeBytes = append(runeBytes, input[0])
				if utf8.FullRune(runeBytes) {
					r, _ := utf8.DecodeRune(runeBytes)
					query += string(r)
					runeBytes = runeBytes[:0]
					selected = 0
				}
			}
		}
	}
}

func filterPickerHosts(hosts []config.Host, query string) []config.Host {
	terms := strings.Fields(strings.ToLower(query))
	matches := make([]config.Host, 0, len(hosts))
	for _, host := range hosts {
		text := strings.ToLower(strings.Join([]string{
			host.Alias, host.User, host.Host, host.Note, strings.Join(host.Tags, " "),
		}, " "))
		matched := true
		for _, term := range terms {
			if !strings.Contains(text, term) {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, host)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Pinned != matches[j].Pinned {
			return matches[i].Pinned
		}
		if matches[i].LastUsedAt != matches[j].LastUsedAt {
			return matches[i].LastUsedAt > matches[j].LastUsedAt
		}
		return matches[i].Alias < matches[j].Alias
	})
	return matches
}

func renderHostPicker(hosts []config.Host, query string, selected int) {
	renderHostPickerTo(os.Stderr, hosts, query, selected, pickerTerminalWidth())
}

func renderHostPickerTo(w io.Writer, hosts []config.Host, query string, selected, width int) {
	fmt.Fprint(w, "\033[2J\033[H")
	fmt.Fprintf(w, "%s\r\n", Header("sshm 主机选择器"))
	fmt.Fprintf(w, "  搜索: %s\r\n", query)
	fmt.Fprint(w, "  输入过滤 / ↑↓选择 / Enter连接 / q或Ctrl+C进入命令模式\r\n\r\n")
	if len(hosts) == 0 {
		fmt.Fprint(w, "  (没有匹配主机)\r\n")
		return
	}
	limit := min(12, len(hosts))
	start := 0
	if selected >= limit {
		start = selected - limit + 1
	}
	end := min(start+limit, len(hosts))
	contentWidth := max(2, width-8)
	for i := start; i < end; i++ {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		host := hosts[i]
		label := fmt.Sprintf("%-18s %s@%s:%d", displayAlias(host), host.User, host.Host, host.Port)
		if len(host.Tags) > 0 {
			label += "  [" + strings.Join(host.Tags, ",") + "]"
		}
		fmt.Fprintf(w, "%s%s\r\n", prefix, truncateToWidth(label, contentWidth))
	}
	if len(hosts) > limit {
		fmt.Fprintf(w, "\r\n  显示 %d-%d / %d\r\n", start+1, end, len(hosts))
	}
}

func clearPicker() {
	fmt.Fprint(os.Stderr, "\033[2J\033[H")
}

func pickerTerminalWidth() int {
	if width, _, err := term.GetSize(int(os.Stdin.Fd())); err == nil && width > 0 {
		return width
	}
	return 100
}
