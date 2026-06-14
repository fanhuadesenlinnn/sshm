package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
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
	if displayWidth(s) <= maxWidth {
		return s
	}
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
	Width   int
}

func RenderHostsTableWithOptions(hosts []config.Host, options HostTableOptions) {
	renderHostsTableTo(os.Stdout, hosts, options)
}

type hostTableColumn struct {
	header     string
	values     []string
	width      int
	minWidth   int
	optional   bool
	shrinkRank int
}

func renderHostsTableTo(w io.Writer, hosts []config.Host, options HostTableOptions) {
	if len(hosts) == 0 {
		fmt.Fprintln(w, "  (暂无主机)")
		return
	}

	ids := make([]string, len(hosts))
	aliases := make([]string, len(hosts))
	targets := make([]string, len(hosts))
	tags := make([]string, len(hosts))
	notes := make([]string, len(hosts))
	auth := make([]string, len(hosts))
	trust := make([]string, len(hosts))
	jumps := make([]string, len(hosts))
	for i, host := range hosts {
		ids[i] = displayHostID(i, options.Indices)
		aliases[i] = displayAlias(host)
		targets[i] = displayConnectionTarget(host)
		tags[i] = config.TagsToString(host.Tags)
		notes[i] = host.Note
		auth[i] = host.Auth
		trust[i] = displayHostTrust(host)
		jumps[i] = host.JumpHost
	}

	columns := []hostTableColumn{
		newHostTableColumn("ID", ids, 2, 6, false, 0),
		newHostTableColumn("主机", aliases, 6, 28, false, 2),
		newHostTableColumn("连接目标", targets, 12, 48, false, 1),
	}
	if options.Wide {
		columns = append(columns,
			newHostTableColumn("认证", auth, 4, 12, false, 6),
			newHostTableColumn("信任", trust, 6, 14, false, 5),
			newHostTableColumn("跳板机", jumps, 6, 24, false, 4),
			newHostTableColumn("标签", tags, 6, 28, false, 3),
			newHostTableColumn("备注", notes, 8, 40, false, 2),
		)
	} else if !options.Compact {
		if hasNonEmpty(tags) {
			columns = append(columns, newHostTableColumn("标签", tags, 6, 24, true, 3))
		}
		if hasNonEmpty(notes) {
			columns = append(columns, newHostTableColumn("备注", notes, 8, 36, true, 2))
		}
	}

	width := options.Width
	if width <= 0 {
		width = terminalWidth()
	}
	columns = fitHostTableColumns(columns, width, options.Wide)

	fmt.Fprintln(w)
	if !options.Compact {
		renderHostTableRow(w, columns, -1)
		fmt.Fprintln(w, "  "+strings.Repeat("-", hostTableContentWidth(columns)))
	}
	for row := range hosts {
		renderHostTableRow(w, columns, row)
	}
	fmt.Fprintln(w)
}

func newHostTableColumn(header string, values []string, minWidth, maxWidth int, optional bool, shrinkRank int) hostTableColumn {
	width := displayWidth(header)
	for _, value := range values {
		width = max(width, displayWidth(value))
	}
	width = min(width, maxWidth)
	return hostTableColumn{
		header: header, values: values, width: width,
		minWidth: min(minWidth, width),
		optional: optional, shrinkRank: shrinkRank,
	}
}

func fitHostTableColumns(columns []hostTableColumn, terminalWidth int, wide bool) []hostTableColumn {
	for !wide && hostTableContentWidth(columns)+2 > terminalWidth {
		dropped := false
		for i := len(columns) - 1; i >= 0; i-- {
			if columns[i].optional {
				columns = append(columns[:i], columns[i+1:]...)
				dropped = true
				break
			}
		}
		if !dropped {
			break
		}
	}
	for hostTableContentWidth(columns)+2 > terminalWidth {
		best := -1
		for i := range columns {
			if columns[i].width <= columns[i].minWidth {
				continue
			}
			if best == -1 || columns[i].shrinkRank < columns[best].shrinkRank {
				best = i
			}
		}
		if best == -1 {
			break
		}
		columns[best].width--
	}
	return columns
}

func renderHostTableRow(w io.Writer, columns []hostTableColumn, row int) {
	fmt.Fprint(w, "  ")
	for i, column := range columns {
		text := column.header
		if row >= 0 {
			text = column.values[row]
		}
		if i == len(columns)-1 {
			fmt.Fprint(w, truncateToWidth(text, column.width))
		} else {
			fmt.Fprint(w, padToWidth(text, column.width), " ")
		}
	}
	fmt.Fprintln(w)
}

func hostTableContentWidth(columns []hostTableColumn) int {
	width := max(0, len(columns)-1)
	for _, column := range columns {
		width += column.width
	}
	return width
}

func hasNonEmpty(values []string) bool {
	for _, value := range values {
		if value != "" {
			return true
		}
	}
	return false
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

func displayConnectionTarget(h config.Host) string {
	host := h.Host
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	target := h.User + "@" + host
	if h.Port != 0 && h.Port != 22 {
		target += fmt.Sprintf(":%d", h.Port)
	}
	return target
}

func displayHostTrust(h config.Host) string {
	if h.ResolvedHostKeyPolicy != "" {
		return h.ResolvedHostKeyPolicy
	}
	if h.HostKeyPolicy != "" {
		return h.HostKeyPolicy
	}
	return config.HostKeyPolicyStrict
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
	policy := h.ResolvedHostKeyPolicy
	if policy == "" {
		policy = h.HostKeyPolicy
	}
	if policy == "" {
		policy = config.HostKeyPolicyStrict
	}
	source := "全局"
	if h.HostKeyPolicy != "" {
		source = "主机覆盖"
	}
	fmt.Printf("  %s %s (%s)\n", BoldText("主机信任:"), policy, source)
	if h.JumpHost != "" {
		fmt.Printf("  %s %s\n", BoldText("跳板机:"), h.JumpHost)
	}
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

func RenderTagsTable(tags []config.Tag, hosts []config.Host) {
	if len(tags) == 0 {
		fmt.Println("  (暂无标签)")
		fmt.Println()
		return
	}
	names := make([]string, len(tags))
	counts := make([]string, len(tags))
	notes := make([]string, len(tags))
	examples := make([]string, len(tags))
	for i, tag := range tags {
		names[i] = tag.Name
		notes[i] = tag.Note
		var aliases []string
		for _, host := range hosts {
			if host.HasTag(tag.Name) {
				aliases = append(aliases, host.Alias)
			}
		}
		counts[i] = fmt.Sprintf("%d", len(aliases))
		if len(aliases) > 3 {
			examples[i] = strings.Join(aliases[:3], ", ") + "…"
		} else {
			examples[i] = strings.Join(aliases, ", ")
		}
	}
	columns := []hostTableColumn{
		newHostTableColumn("标签", names, 6, 28, false, 3),
		newHostTableColumn("主机数", counts, 6, 8, false, 0),
	}
	if hasNonEmpty(notes) {
		columns = append(columns, newHostTableColumn("备注", notes, 8, 36, true, 2))
	}
	if hasNonEmpty(examples) {
		columns = append(columns, newHostTableColumn("示例主机", examples, 8, 48, true, 1))
	}
	columns = fitHostTableColumns(columns, terminalWidth(), false)
	fmt.Println()
	renderHostTableRow(os.Stdout, columns, -1)
	fmt.Println("  " + strings.Repeat("-", hostTableContentWidth(columns)))
	for row := range tags {
		renderHostTableRow(os.Stdout, columns, row)
	}
	fmt.Println()
}
