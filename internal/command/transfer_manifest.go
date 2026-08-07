package command

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/pkg/sftp"
)

type manifestEntry struct {
	Path   string
	Type   string
	Mode   os.FileMode
	SHA256 string
}

func localManifest(root string) ([]manifestEntry, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("读取本地源失败: %w", err)
	}
	var entries []manifestEntry
	var walk func(string, string, os.FileInfo) error
	walk = func(current, relative string, info os.FileInfo) error {
		entry, err := localManifestEntry(current, relative, info)
		if err != nil {
			return err
		}
		entries = append(entries, entry)
		if !info.IsDir() {
			return nil
		}
		children, err := os.ReadDir(current)
		if err != nil {
			return fmt.Errorf("读取本地目录 %s 失败: %w", current, err)
		}
		for _, child := range children {
			childPath := filepath.Join(current, child.Name())
			childInfo, err := os.Lstat(childPath)
			if err != nil {
				return err
			}
			childRelative := filepath.ToSlash(filepath.Join(relative, child.Name()))
			if relative == "." {
				childRelative = child.Name()
			}
			if err := walk(childPath, childRelative, childInfo); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, ".", info); err != nil {
		return nil, err
	}
	sortManifest(entries)
	return entries, nil
}

func localManifestEntry(current, relative string, info os.FileInfo) (manifestEntry, error) {
	entry := manifestEntry{Path: relative, Mode: info.Mode().Perm()}
	switch {
	case info.IsDir():
		entry.Type = "dir"
	case info.Mode().IsRegular():
		entry.Type = "file"
		checksum, err := localSHA256(current)
		if err != nil {
			return manifestEntry{}, err
		}
		entry.SHA256 = checksum
	default:
		return manifestEntry{}, fmt.Errorf("不支持符号链接或特殊文件: %s", current)
	}
	return entry, nil
}

func remoteManifest(client *sftp.Client, root string) ([]manifestEntry, error) {
	info, err := client.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("读取远程路径 %s 失败: %w", root, err)
	}
	var entries []manifestEntry
	var walk func(string, string, os.FileInfo) error
	walk = func(current, relative string, info os.FileInfo) error {
		entry := manifestEntry{Path: relative, Mode: info.Mode().Perm()}
		switch {
		case info.IsDir():
			entry.Type = "dir"
		case info.Mode().IsRegular():
			entry.Type = "file"
			checksum, err := remoteSHA256(client, current)
			if err != nil {
				return err
			}
			entry.SHA256 = checksum
		default:
			return fmt.Errorf("不支持远程符号链接或特殊文件: %s", current)
		}
		entries = append(entries, entry)
		if !info.IsDir() {
			return nil
		}
		children, err := client.ReadDir(current)
		if err != nil {
			return fmt.Errorf("读取远程目录 %s 失败: %w", current, err)
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			name, err := safeRemoteEntryName(child.Name())
			if err != nil {
				return err
			}
			childPath := path.Join(current, name)
			childInfo, err := client.Lstat(childPath)
			if err != nil {
				return err
			}
			childRelative := path.Join(relative, name)
			if relative == "." {
				childRelative = name
			}
			if err := walk(childPath, childRelative, childInfo); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, ".", info); err != nil {
		return nil, err
	}
	sortManifest(entries)
	return entries, nil
}

func localSHA256(file string) (string, error) {
	reader, err := os.Open(file)
	if err != nil {
		return "", fmt.Errorf("读取本地文件 %s 失败: %w", file, err)
	}
	defer reader.Close()
	return hashSHA256(reader)
}

func remoteSHA256(client *sftp.Client, file string) (string, error) {
	reader, err := client.Open(file)
	if err != nil {
		return "", fmt.Errorf("读取远程文件 %s 失败: %w", file, err)
	}
	defer reader.Close()
	return hashSHA256(reader)
}

func hashSHA256(reader io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func manifestsEqual(left, right []manifestEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Path != right[i].Path || left[i].Type != right[i].Type || left[i].SHA256 != right[i].SHA256 {
			return false
		}
	}
	return true
}

func sortManifest(entries []manifestEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
}
