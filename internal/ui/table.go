package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"golang.org/x/term"
)

// displayWidth returns the terminal display width of a string.
// CJK characters and other wide characters count as 2, ASCII as 1.
// ANSI escape sequences are ignored.
func displayWidth(s string) int {
	width := 0
	inEscape := false
	for _, r := range s {
		if inEscape {
			if r == 'm' || r == 'K' || r == 'G' || r == 'H' || r == 'J' || r == 'A' || r == 'B' || r == 'C' || r == 'D' {
				inEscape = false
			}
			continue
		}
		if r == '\033' {
			inEscape = true
			continue
		}
		width += runeDisplayWidth(r)
	}
	return width
}

// runeDisplayWidth returns the display width of a single rune (no ANSI).
func runeDisplayWidth(r rune) int {
	if r < 32 {
		return 0
	}
	if r < 0x7f {
		return 1
	}
	// CJK Unified Ideographs
	if r >= 0x4e00 && r <= 0x9fff {
		return 2
	}
	// CJK Unified Ideographs Extension A
	if r >= 0x3400 && r <= 0x4dbf {
		return 2
	}
	// CJK Compatibility Ideographs
	if r >= 0xf900 && r <= 0xfaff {
		return 2
	}
	// CJK Symbols and Punctuation, Hiragana, Katakana, Bopomofo, Hangul, etc.
	if r >= 0x2e80 && r <= 0x33bf {
		return 2
	}
	// Fullwidth Forms
	if r >= 0xff01 && r <= 0xff60 {
		return 2
	}
	// Fullwidth/Halfwidth Symbol Variants
	if r >= 0xffe0 && r <= 0xffe6 {
		return 2
	}
	// Hangul Syllables
	if r >= 0xac00 && r <= 0xd7af {
		return 2
	}
	// CJK Extensions B-F and Supplement
	if r >= 0x20000 && r <= 0x2ffff {
		return 2
	}
	// Emoji and other wide symbols
	if r >= 0x1f000 && r <= 0x1ffff {
		return 2
	}
	return 1
}

// padToWidth pads a string to the given display width.
// If the string is longer, it is truncated with an ellipsis.
func padToWidth(s string, width int) string {
	cur := displayWidth(s)
	if cur > width {
		// Truncate to fit, append "…"
		return truncateToWidth(s, width) + strings.Repeat(" ", width-displayWidth(truncateToWidth(s, width)))
	}
	return s + strings.Repeat(" ", width-cur)
}

// truncateToWidth truncates a string to fit within display width, adding "…".
func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 1 {
		return ""
	}
	w := 0
	runes := []rune(s)
	for i, r := range runes {
		rw := runeDisplayWidth(r)
		if w+rw+1 > maxWidth { // +1 for "…"
			return string(runes[:i]) + "…"
		}
		w += rw
	}
	return s
}

// RenderHostsTable renders a list of hosts as a formatted table.
func RenderHostsTable(hosts []config.Host) {
	RenderHostsTableWithOptions(hosts, HostTableOptions{})
}

// HostTableOptions controls host table layout and displayed IDs.
type HostTableOptions struct {
	Indices []int
	Compact bool
	Wide    bool
}

// RenderHostsTableWithOptions renders hosts using terminal-aware column widths.
func RenderHostsTableWithOptions(hosts []config.Host, options HostTableOptions) {
	if len(hosts) == 0 {
		fmt.Println("  (暂无主机)")
		return
	}

	colID := 4
	colAlias := 18
	colAddr := 24
	colPort := 6
	colGroup := 12
	colAuth := 10
	colNote := 18
	width := terminalWidth()
	if options.Wide {
		colAlias, colAddr, colGroup, colNote = 28, 40, 20, 40
	} else if width > 100 {
		extra := width - 100
		colAddr += extra / 2
		colNote += extra - extra/2
	}
	compact := options.Compact || width < 90

	fmt.Println()
	if compact {
		fmt.Printf("  %s %s %s %s\n",
			padToWidth("ID", colID),
			padToWidth("别名", colAlias),
			padToWidth("用户@主机", colAddr),
			padToWidth("分组", colGroup))
		sepWidth := 2 + colID + 1 + colAlias + 1 + colAddr + 1 + colGroup
		fmt.Println("  " + strings.Repeat("-", sepWidth-2))
		for i, h := range hosts {
			fmt.Printf("  %s %s %s %s\n",
				padToWidth(displayHostID(i, options.Indices), colID),
				padToWidth(displayAlias(h), colAlias),
				padToWidth(h.User+"@"+h.Host, colAddr),
				padToWidth(h.Group, colGroup))
		}
		fmt.Println()
		return
	}

	fmt.Printf("  %s %s %s %s %s %s %s\n",
		padToWidth("ID", colID),
		padToWidth("别名", colAlias),
		padToWidth("用户@主机", colAddr),
		padToWidth("端口", colPort),
		padToWidth("分组", colGroup),
		padToWidth("认证", colAuth),
		padToWidth("备注", colNote))

	sepWidth := 2 + colID + 1 + colAlias + 1 + colAddr + 1 + colPort + 1 + colGroup + 1 + colAuth + 1 + colNote
	fmt.Println("  " + strings.Repeat("-", sepWidth-2))

	for i, h := range hosts {
		port := fmt.Sprintf("%d", h.Port)

		fmt.Printf("  %s %s %s %s %s %s %s\n",
			padToWidth(displayHostID(i, options.Indices), colID),
			padToWidth(displayAlias(h), colAlias),
			padToWidth(h.User+"@"+h.Host, colAddr),
			padToWidth(port, colPort),
			padToWidth(h.Group, colGroup),
			padToWidth(h.Auth, colAuth),
			padToWidth(h.Note, colNote))
	}
	fmt.Println()
}

func displayAlias(h config.Host) string {
	if h.Pinned {
		return "* " + h.Alias
	}
	return h.Alias
}

func displayHostID(position int, indices []int) string {
	if position < len(indices) {
		return fmt.Sprintf("%d", indices[position]+1)
	}
	return fmt.Sprintf("%d", position+1)
}

func terminalWidth() int {
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}
	return 100
}

// RenderHostDetail renders detailed info for a single host.
func RenderHostDetail(h config.Host, index int) {
	fmt.Println()
	fmt.Println(Header("主机详情"))
	fmt.Println("  " + strings.Repeat("-", 40))
	fmt.Printf("  %s %s\n", BoldText("ID:"), fmt.Sprintf("%d", index+1))
	fmt.Printf("  %s %s\n", BoldText("别名:"), h.Alias)
	if h.Pinned {
		fmt.Printf("  %s %s\n", BoldText("收藏:"), "是")
	}
	fmt.Printf("  %s %s\n", BoldText("用户:"), h.User)
	fmt.Printf("  %s %s\n", BoldText("主机:"), h.Host)
	fmt.Printf("  %s %d\n", BoldText("端口:"), h.Port)
	if h.Identity != "" {
		fmt.Printf("  %s %s\n", BoldText("密钥:"), h.Identity)
	} else {
		fmt.Printf("  %s %s\n", BoldText("密钥:"), DimText("无"))
	}
	if h.PasswordRef != "" {
		fmt.Printf("  %s %s\n", BoldText("密码:"), "已保存")
	} else {
		fmt.Printf("  %s %s\n", BoldText("密码:"), DimText("未保存"))
	}
	fmt.Printf("  %s %s\n", BoldText("认证:"), h.Auth)
	if h.Note != "" {
		fmt.Printf("  %s %s\n", BoldText("备注:"), h.Note)
	}
	if h.Group != "" {
		fmt.Printf("  %s %s\n", BoldText("分组:"), h.Group)
	}
	if len(h.Tags) > 0 {
		fmt.Printf("  %s %s\n", BoldText("标签:"), strings.Join(h.Tags, ", "))
	}
	if h.LastUsedAt != "" {
		fmt.Printf("  %s %s\n", BoldText("最近连接:"), h.LastUsedAt)
	}
	fmt.Println()
}

// RenderSearchResults renders search results.
func RenderSearchResults(hosts []config.Host, indices []int, keyword string) {
	if len(hosts) == 0 {
		fmt.Println("  (未找到匹配主机)")
		return
	}
	fmt.Println()
	fmt.Printf("  搜索 '%s' 结果 (%d 个):\n", keyword, len(hosts))

	RenderHostsTableWithOptions(hosts, HostTableOptions{Indices: indices})
}

// RenderGroupList renders a list of groups with host counts.
func RenderGroupList(groups map[string][]config.Host) {
	if len(groups) == 0 {
		fmt.Println("  (暂无分组)")
		return
	}
	fmt.Println()
	fmt.Println(Header("分组列表"))
	fmt.Println()

	names := make([]string, 0, len(groups))
	for group := range groups {
		names = append(names, group)
	}
	sort.Strings(names)
	for _, group := range names {
		hosts := groups[group]
		sort.SliceStable(hosts, func(i, j int) bool {
			return hosts[i].Alias < hosts[j].Alias
		})
		displayGroup := group
		if displayGroup == "" {
			displayGroup = "(未分组)"
		}
		fmt.Printf("  %s%s%s (%d 台)\n", Bold, displayGroup, Reset, len(hosts))
		for _, h := range hosts {
			fmt.Printf("    - %s (%s@%s:%d)\n", h.Alias, h.User, h.Host, h.Port)
		}
		fmt.Println()
	}
}
