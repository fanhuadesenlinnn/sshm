package ui

import (
	"fmt"
	"strings"

	"github.com/sshm/sshm/internal/config"
)

// RenderHostsTable renders a list of hosts as a formatted table.
func RenderHostsTable(hosts []config.Host) {
	if len(hosts) == 0 {
		fmt.Println("  (暂无主机)")
		return
	}

	// Header
	fmt.Println()
	fmt.Printf("  %-4s %-18s %-14s %-5s %-10s %-12s %s\n",
		"ID", "别名", "用户@主机", "端口", "分组", "认证", "备注")
	fmt.Println("  " + strings.Repeat("-", 78))

	for i, h := range hosts {
		addr := truncate(h.User+"@"+h.Host, 14)
		note := truncate(h.Note, 18)
		group := truncate(h.Group, 10)
		auth := h.Auth
		if auth == "auto" {
			auth = "auto"
		}

		fmt.Printf("  %-4d %-18s %-14s %-5d %-10s %-12s %s\n",
			i+1, truncate(h.Alias, 18), addr, h.Port, group, auth, note)
	}
	fmt.Println()
}

// RenderHostDetail renders detailed info for a single host.
func RenderHostDetail(h config.Host, index int) {
	fmt.Println()
	fmt.Println(Header("主机详情"))
	fmt.Println("  " + strings.Repeat("-", 40))
	fmt.Printf("  %s %s\n", BoldText("ID:"), fmt.Sprintf("%d", index+1))
	fmt.Printf("  %s %s\n", BoldText("别名:"), h.Alias)
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
	fmt.Println()
}

// RenderSearchResults renders search results.
func RenderSearchResults(hosts []config.Host, keyword string) {
	if len(hosts) == 0 {
		fmt.Println("  (未找到匹配主机)")
		return
	}
	fmt.Println()
	fmt.Printf("  搜索 '%s' 结果 (%d 个):\n", keyword, len(hosts))
	fmt.Println()
	fmt.Printf("  %-4s %-18s %-14s %-5s %-10s %s\n",
		"ID", "别名", "用户@主机", "端口", "分组", "备注")
	fmt.Println("  " + strings.Repeat("-", 73))

	for i, h := range hosts {
		addr := truncate(h.User+"@"+h.Host, 14)
		note := truncate(h.Note, 18)
		group := truncate(h.Group, 10)

		fmt.Printf("  %-4d %-18s %-14s %-5d %-10s %s\n",
			i+1, truncate(h.Alias, 18), addr, h.Port, group, note)
	}
	fmt.Println()
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

	for group, hosts := range groups {
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

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}


