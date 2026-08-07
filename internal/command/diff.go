package command

import "strings"

// maxDiffMatrixCells caps the LCS work per file; larger files fall back to a
// full replace listing instead of consuming unbounded memory.
const maxDiffMatrixCells = 4_000_000

// diffContextLines is the number of unchanged lines kept around each hunk.
const diffContextLines = 3

type diffOp struct {
	kind     byte // ' ', '-', '+'
	oldIndex int  // -1 for insertions
	newIndex int  // -1 for deletions
}

// unifiedDiff returns a line-based unified diff of two texts.
func unifiedDiff(oldText, newText string) string {
	oldLines := splitDiffLines(oldText)
	newLines := splitDiffLines(newText)
	var ops []diffOp
	if len(oldLines)*len(newLines) <= maxDiffMatrixCells {
		ops = lcsDiffOps(oldLines, newLines)
	} else {
		ops = replaceAllOps(oldLines, newLines)
	}
	return formatUnifiedDiff(ops, oldLines, newLines)
}

func splitDiffLines(data string) []string {
	value := strings.TrimSuffix(data, "\n")
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}

func replaceAllOps(oldLines, newLines []string) []diffOp {
	ops := make([]diffOp, 0, len(oldLines)+len(newLines))
	for i := range oldLines {
		ops = append(ops, diffOp{kind: '-', oldIndex: i, newIndex: -1})
	}
	for j := range newLines {
		ops = append(ops, diffOp{kind: '+', oldIndex: -1, newIndex: j})
	}
	return ops
}

// lcsDiffOps computes an edit script via longest-common-subsequence dynamic
// programming.
func lcsDiffOps(oldLines, newLines []string) []diffOp {
	n, m := len(oldLines), len(newLines)
	// moves[i][j]: 0 = match, 1 = delete old, 2 = insert new.
	moves := make([][]byte, n+1)
	for i := range moves {
		moves[i] = make([]byte, m+1)
		moves[i][0] = 1
	}
	for j := range moves[0] {
		moves[0][j] = 2
	}
	prev := make([]int, m+1)
	curr := make([]int, m+1)
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if oldLines[i-1] == newLines[j-1] {
				curr[j] = prev[j-1] + 1
				moves[i][j] = 0
			} else if prev[j] >= curr[j-1] {
				curr[j] = prev[j]
				moves[i][j] = 1
			} else {
				curr[j] = curr[j-1]
				moves[i][j] = 2
			}
		}
		prev, curr = curr, prev
	}
	var ops []diffOp
	i, j := n, m
	for i > 0 || j > 0 {
		switch moves[i][j] {
		case 0:
			ops = append(ops, diffOp{kind: ' ', oldIndex: i - 1, newIndex: j - 1})
			i--
			j--
		case 1:
			ops = append(ops, diffOp{kind: '-', oldIndex: i - 1, newIndex: -1})
			i--
		case 2:
			ops = append(ops, diffOp{kind: '+', oldIndex: -1, newIndex: j - 1})
			j--
		}
	}
	for left, right := 0, len(ops)-1; left < right; left, right = left+1, right-1 {
		ops[left], ops[right] = ops[right], ops[left]
	}
	return ops
}

type displayDiffLine struct {
	kind  byte
	text  string
	oldNo int // 1-based old line number, 0 when absent
	newNo int // 1-based new line number, 0 when absent
}

func formatUnifiedDiff(ops []diffOp, oldLines, newLines []string) string {
	display := make([]displayDiffLine, 0, len(ops))
	oldNo, newNo := 0, 0
	for _, op := range ops {
		line := displayDiffLine{kind: op.kind}
		switch op.kind {
		case ' ':
			oldNo++
			newNo++
			line.oldNo, line.newNo = oldNo, newNo
			line.text = oldLines[op.oldIndex]
		case '-':
			oldNo++
			line.oldNo = oldNo
			line.text = oldLines[op.oldIndex]
		case '+':
			newNo++
			line.newNo = newNo
			line.text = newLines[op.newIndex]
		}
		display = append(display, line)
	}
	if len(display) == 0 {
		return ""
	}
	changed := false
	for _, op := range ops {
		if op.kind != ' ' {
			changed = true
			break
		}
	}
	if !changed {
		return ""
	}
	// 规范化：同一变更区间内先删除后插入（- 在前 + 在后）。
	for i := 0; i < len(display); {
		if display[i].kind == ' ' {
			i++
			continue
		}
		j := i
		for j < len(display) && display[j].kind != ' ' {
			j++
		}
		run := display[i:j]
		ordered := make([]displayDiffLine, 0, len(run))
		for _, line := range run {
			if line.kind == '-' {
				ordered = append(ordered, line)
			}
		}
		for _, line := range run {
			if line.kind == '+' {
				ordered = append(ordered, line)
			}
		}
		copy(run, ordered)
		i = j
	}

	var hunks [][]displayDiffLine
	var current []displayDiffLine
	lastChange := -1
	for idx := range display {
		if display[idx].kind != ' ' {
			if lastChange >= 0 && idx-lastChange-1 > 2*diffContextLines {
				hunks = append(hunks, current)
				current = nil
				lastChange = -1
			}
			lastChange = idx
		}
		current = append(current, display[idx])
	}
	if len(current) > 0 {
		hunks = append(hunks, current)
	}

	var builder strings.Builder
	for _, hunk := range hunks {
		trimDiffContext(&hunk)
		oldStart, newStart := 0, 0
		oldCount, newCount := 0, 0
		for _, line := range hunk {
			if line.oldNo > 0 {
				if oldStart == 0 {
					oldStart = line.oldNo
				}
				oldCount++
			}
			if line.newNo > 0 {
				if newStart == 0 {
					newStart = line.newNo
				}
				newCount++
			}
		}
		if oldCount == 0 {
			oldStart = max(0, newStart-1)
		}
		if newCount == 0 {
			newStart = max(0, oldStart-1)
		}
		builder.WriteString("@@ -")
		builder.WriteString(itoa(oldStart))
		builder.WriteByte(',')
		builder.WriteString(itoa(oldCount))
		builder.WriteString(" +")
		builder.WriteString(itoa(newStart))
		builder.WriteByte(',')
		builder.WriteString(itoa(newCount))
		builder.WriteString(" @@\n")
		for _, line := range hunk {
			builder.WriteByte(line.kind)
			builder.WriteString(line.text)
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

func trimDiffContext(hunk *[]displayDiffLine) {
	lines := *hunk
	firstChange := 0
	for firstChange < len(lines) && lines[firstChange].kind == ' ' {
		firstChange++
	}
	lastChange := len(lines) - 1
	for lastChange >= 0 && lines[lastChange].kind == ' ' {
		lastChange--
	}
	start := max(0, firstChange-diffContextLines)
	end := min(len(lines), lastChange+1+diffContextLines)
	*hunk = lines[start:end]
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buffer [12]byte
	index := len(buffer)
	negative := value < 0
	if negative {
		value = -value
	}
	for value > 0 {
		index--
		buffer[index] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		index--
		buffer[index] = '-'
	}
	return string(buffer[index:])
}
