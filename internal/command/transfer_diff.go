package command

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"unicode/utf8"

	"github.com/pkg/sftp"
)

const maxDiffFileSize = 1024 * 1024

func writePushDiff(
	writer io.Writer,
	client *sftp.Client,
	localRoot, remoteRoot string,
	source, target []manifestEntry,
) error {
	targetByPath := make(map[string]manifestEntry, len(target))
	for _, entry := range target {
		targetByPath[entry.Path] = entry
	}
	sourceByPath := make(map[string]manifestEntry, len(source))
	for _, entry := range source {
		sourceByPath[entry.Path] = entry
		if entry.Type != "file" {
			continue
		}
		if previous, ok := targetByPath[entry.Path]; ok && previous == entry {
			continue
		}
		localFile := localRoot
		remoteFile := remoteRoot
		if entry.Path != "." {
			localFile = filepath.Join(localRoot, filepath.FromSlash(entry.Path))
			remoteFile = path.Join(remoteRoot, entry.Path)
		}
		if err := writeFileDiff(writer, client, localFile, remoteFile, targetByPath[entry.Path], entry); err != nil {
			return err
		}
	}
	for _, entry := range target {
		if _, ok := sourceByPath[entry.Path]; !ok {
			fmt.Fprintf(writer, "deleted: %s\n", entry.Path)
		}
	}
	return nil
}

func writeFileDiff(writer io.Writer, client *sftp.Client, localFile, remoteFile string, oldEntry, newEntry manifestEntry) error {
	newData, newText, err := readLocalDiffFile(localFile)
	if err != nil {
		return err
	}
	var oldData []byte
	oldText := true
	if oldEntry.Type == "file" {
		oldData, oldText, err = readRemoteDiffFile(client, remoteFile)
		if err != nil {
			return err
		}
	}
	if !newText || !oldText {
		oldChecksum := oldEntry.SHA256
		if oldChecksum == "" {
			oldChecksum = "<missing>"
		}
		fmt.Fprintf(writer, "binary differs: %s sha256 %s -> %s\n", remoteFile, oldChecksum, newEntry.SHA256)
		return nil
	}
	fmt.Fprintf(writer, "--- %s\n+++ %s\n", remoteFile, localFile)
	fmt.Fprint(writer, unifiedDiff(string(oldData), string(newData)))
	return nil
}

func readLocalDiffFile(file string) ([]byte, bool, error) {
	reader, err := os.Open(file)
	if err != nil {
		return nil, false, fmt.Errorf("读取本地 diff 文件失败: %w", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxDiffFileSize+1))
	if err != nil {
		return nil, false, fmt.Errorf("读取本地 diff 文件失败: %w", err)
	}
	return classifyDiffData(data)
}

func readRemoteDiffFile(client *sftp.Client, file string) ([]byte, bool, error) {
	reader, err := client.Open(file)
	if err != nil {
		return nil, false, fmt.Errorf("读取远程 diff 文件失败: %w", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxDiffFileSize+1))
	if err != nil {
		return nil, false, err
	}
	return classifyDiffData(data)
}

func classifyDiffData(data []byte) ([]byte, bool, error) {
	if len(data) > maxDiffFileSize {
		return data[:maxDiffFileSize], false, nil
	}
	return data, utf8.Valid(data) && !bytes.ContainsRune(data, '\x00'), nil
}
