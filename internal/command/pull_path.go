package command

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
)

func multiPullDestination(localDest, hostAlias, remotePath string, flat bool) (string, error) {
	if err := config.ValidateHostAlias(hostAlias); err != nil {
		return "", fmt.Errorf("主机别名不能安全用于本地路径: %w", err)
	}
	parts, err := safeRemotePathParts(remotePath, runtime.GOOS == "windows")
	if err != nil {
		return "", err
	}
	root := filepath.Clean(localDest)
	if flat {
		return confinedJoin(root, parts[len(parts)-1])
	}
	hostRoot, err := confinedJoin(root, hostAlias)
	if err != nil {
		return "", err
	}
	return confinedJoin(hostRoot, filepath.Join(parts...))
}

func safeRemotePathParts(remotePath string, windows bool) ([]string, error) {
	if remotePath == "" || strings.ContainsRune(remotePath, '\x00') {
		return nil, fmt.Errorf("远程路径不能为空或包含 NUL")
	}
	for _, component := range strings.Split(remotePath, "/") {
		if component == ".." {
			return nil, fmt.Errorf("远程路径包含不安全的 ..: %s", remotePath)
		}
	}
	clean := path.Clean(remotePath)
	if clean == "." || clean == "/" {
		return nil, fmt.Errorf("远程路径必须指向明确文件或目录")
	}
	clean = strings.TrimPrefix(clean, "/")
	parts := strings.Split(clean, "/")
	for _, component := range parts {
		if component == "" || component == "." || component == ".." {
			return nil, fmt.Errorf("远程路径包含不安全组件: %s", remotePath)
		}
		if err := validateLocalComponent(component, windows); err != nil {
			return nil, fmt.Errorf("远程路径组件 %q 无法安全保存: %w", component, err)
		}
	}
	return parts, nil
}

func validateLocalComponent(component string, windows bool) error {
	if component == "" || component == "." || component == ".." {
		return fmt.Errorf("无效路径组件")
	}
	if strings.ContainsAny(component, `/\`) || strings.ContainsRune(component, '\x00') {
		return fmt.Errorf("包含路径分隔符或 NUL")
	}
	if !windows {
		return nil
	}
	if strings.ContainsAny(component, `<>:"|?*`) {
		return fmt.Errorf("包含 Windows 非法字符")
	}
	if strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") {
		return fmt.Errorf("不能以点或空格结尾")
	}
	base := strings.ToUpper(strings.TrimSuffix(component, filepath.Ext(component)))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return fmt.Errorf("使用 Windows 保留名称")
	}
	return nil
}

func confinedJoin(root string, components ...string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(append([]string{root}, components...)...)
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("目标路径逃逸根目录: %s", target)
	}
	if err := rejectSymlinkComponents(root, target); err != nil {
		return "", err
	}
	return target, nil
}

// rejectSymlinkComponents prevents a lexically confined download path from
// escaping through a pre-existing symlink below the selected local root.
// Ancestors above root remain user-controlled and are intentionally allowed.
func rejectSymlinkComponents(root, target string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("目标路径逃逸根目录: %s", target)
	}
	current := root
	parts := []string{}
	if relative != "." {
		parts = strings.Split(relative, string(os.PathSeparator))
	}
	for index := -1; index < len(parts); index++ {
		if index >= 0 {
			current = filepath.Join(current, parts[index])
		}
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			return nil
		}
		if statErr != nil {
			return fmt.Errorf("检查本地目标路径失败: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("本地目标路径包含符号链接，拒绝写入: %s", current)
		}
	}
	return nil
}

func ensureUniqueDestinations(destinations []string, caseInsensitive bool) error {
	seen := map[string]string{}
	for _, destination := range destinations {
		key := filepath.Clean(destination)
		if caseInsensitive {
			key = strings.ToLower(key)
		}
		if previous, ok := seen[key]; ok {
			return fmt.Errorf("本次操作存在目标路径冲突: %s 与 %s", previous, destination)
		}
		seen[key] = destination
	}
	return nil
}

func localPathComparisonCaseInsensitive() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin"
}

func validateRemoteManifestDestinations(destination string, manifest []manifestEntry, windows, caseInsensitive bool) error {
	var destinations []string
	for _, entry := range manifest {
		if entry.Path == "." {
			destinations = append(destinations, destination)
			continue
		}
		components := strings.Split(entry.Path, "/")
		for _, component := range components {
			if err := validateLocalComponent(component, windows); err != nil {
				return fmt.Errorf("远程目录项 %q 无法安全保存: %w", entry.Path, err)
			}
		}
		target, err := confinedJoin(destination, filepath.Join(components...))
		if err != nil {
			return err
		}
		destinations = append(destinations, target)
	}
	return ensureUniqueDestinations(destinations, caseInsensitive)
}
