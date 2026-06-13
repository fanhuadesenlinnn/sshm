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

func runeDisplayWidth(r rune) int {
	if r < 32 {
		return 0
	}
	if r < 0x7f {
		return 1
	}
	if r >= 0x4e00 && r <= 0x9fff {
		return 2
	}
	if r >= 0x3400 && r <= 0x4dbf {
		return 2
	}
	if r >= 0xf900 && r <= 0xfaff {
		return 2
	}
	if r >= 0x2e80 && r <= 0x33bf {
		return 2
	}
	if r >= 0xff01 && r <= 0xff60 {
		return 2
	}
	if r >= 0xffe0 && r <= 0xffe6 {
		return 2
	}
	if r >= 0xac00 && r <= 0xd7af {
		return 2
	}
	if r >= 0x20000 && r <= 0x2ffff {
		return 2
	}
	if r >= 0x1f000 && r <= 0x1ffff {
		return 2
	}
	return 1
}

func padToWidth(s string, width int) string {
	cur := displayWidth(s)
	if cur > width {
		return truncateToWidth(s, width) + strings.Repeat(" ", width-displayWidth(truncateToWidth(s, width)))
	}
	return s + strings.Repeat(" ", width-cur)
}

func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 1 {
		return ""
	}
	w := 0
	runes := []rune(s)
	for i, r := range runes {
		rw := runeDisplayWidth(r)
		if w+rw+1 > maxWidth {
			return string(runes[:i]) + "…"
		}
		w += rw
	}
	return s
}

func RenderHostsTable(hosts []config.Host) {
	RenderHostsTableWithOptions(hosts, HostTableOptions{})
}

type HostTableOptions struct {
	Indices []int
	Compact bool
	Wide    bool
}

func RenderHostsTableWithOptions(hosts []config.Host, options HostTableOptions) {
	if len(hosts) == 0 {
		fmt.Println("  (暂无主机)")
		return
	}

	colID := 4
	colAlias := 18
	colAddr := 24
	colPort := 6
	colTags := 18
	colAuth := 10
	colNote := 18
	width := terminalWidth()
	if options.Wide {
		colAlias, colAddr, colTags, colNote = 28, 40, 28, 40
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
			padToWidth("标签", colTags))
		sepWidth := 2 + colID + 1 + colAlias + 1 + colAddr + 1 + colTags
		fmt.Println("  " + strings.Repeat("-", sepWidth-2))
		for i, h := range hosts {
			fmt.Printf("  %s %s %s %s\n",
				padToWidth(displayHostID(i, options.Indices), colID),
				padToWidth(displayAlias(h), colAlias),
				padToWidth(h.User+"@"+h.Host, colAddr),
				padToWidth(config.TagsToString(h.Tags), colTags))
		}
		fmt.Println()
		return
	}

	fmt.Printf("  %s %s %s %s %s %s %s\n",
		padToWidth("ID", colID),
		padToWidth("别名", colAlias),
		padToWidth("用户@主机", colAddr),
		padToWidth("端口", colPort),
		padToWidth("标签", colTags),
		padToWidth("认证", colAuth),
		padToWidth("备注", colNote))

	sepWidth := 2 + colID + 1 + colAlias + 1 + colAddr + 1 + colPort + 1 + colTags + 1 + colAuth + 1 + colNote
	fmt.Println("  " + strings.Repeat("-", sepWidth-2))

	for i, h := range hosts {
		port := fmt.Sprintf("%d", h.Port)
		fmt.Printf("  %s %s %s %s %s %s %s\n",
			padToWidth(displayHostID(i, options.Indices), colID),
			padToWidth(displayAlias(h), colAlias),
			padToWidth(h.User+"@"+h.Host, colAddr),
			padToWidth(port, colPort),
			padToWidth(config.TagsToString(h.Tags), colTags),
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
	if len(h.Tags) > 0 {
		fmt.Printf("  %s %s\n", BoldText("标签:"), strings.Join(h.Tags, ", "))
	}
	if h.LastUsedAt != "" {
		fmt.Printf("  %s %s\n", BoldText("最近连接:"), h.LastUsedAt)
	}
	fmt.Println()
}

func RenderSearchResults(hosts []config.Host, indices []int, keyword string) {
	if len(hosts) == 0 {
		fmt.Println("  (未找到匹配主机)")
		return
	}
	fmt.Println()
	fmt.Printf("  搜索 '%s' 结果 (%d 个):\n", keyword, len(hosts))
	RenderHostsTableWithOptions(hosts, HostTableOptions{Indices: indices})
}

func RenderTagList(tags map[string][]config.Host) {
	if len(tags) == 0 {
		fmt.Println("  (暂无标签)")
		return
	}
	fmt.Println()
	fmt.Println(Header("标签列表"))
	fmt.Println()

	names := make([]string, 0, len(tags))
	for tag := range tags {
		names = append(names, tag)
	}
	sort.Strings(names)
	for _, tag := range names {
		hosts := tags[tag]
		sort.SliceStable(hosts, func(i, j int) bool {
			return hosts[i].Alias < hosts[j].Alias
		})
		fmt.Printf("  %s%s%s (%d 台)\n", Bold, tag, Reset, len(hosts))
		for _, h := range hosts {
			fmt.Printf("    - %s (%s@%s:%d)\n", h.Alias, h.User, h.Host, h.Port)
		}
		fmt.Println()
	}
}
