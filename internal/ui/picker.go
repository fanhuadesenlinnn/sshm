package ui

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
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
		case backspaceKey, 8:
			runes := []rune(query)
			if len(runes) > 0 {
				query = string(runes[:len(runes)-1])
				selected = 0
			}
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
			host.Alias, host.User, host.Host, host.Group, host.Note, strings.Join(host.Tags, " "),
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
	fmt.Fprint(os.Stderr, "\033[2J\033[H")
	fmt.Fprintln(os.Stderr, Header("sshm 主机选择器"))
	fmt.Fprintf(os.Stderr, "  搜索: %s\n", query)
	fmt.Fprintln(os.Stderr, "  输入过滤 / ↑↓选择 / Enter连接 / q或Ctrl+C进入命令模式")
	fmt.Fprintln(os.Stderr)
	if len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "  (没有匹配主机)")
		return
	}
	limit := min(12, len(hosts))
	width := terminalWidth() - 8
	for i := 0; i < limit; i++ {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		host := hosts[i]
		label := fmt.Sprintf("%-18s %s@%s:%d", displayAlias(host), host.User, host.Host, host.Port)
		if host.Group != "" {
			label += "  [" + host.Group + "]"
		}
		fmt.Fprintf(os.Stderr, "%s%s\n", prefix, truncateToWidth(label, width))
	}
	if len(hosts) > limit {
		fmt.Fprintf(os.Stderr, "\n  另有 %d 台匹配主机，请继续输入缩小范围\n", len(hosts)-limit)
	}
}

func clearPicker() {
	fmt.Fprint(os.Stderr, "\033[2J\033[H")
}
