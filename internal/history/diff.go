package history

import (
	"fmt"
	"strings"
)

// DiffLine represents a single line in a diff output.
type DiffLine struct {
	Type    DiffLineType
	Content string
	OldNum  int // line number in old file (0 if added)
	NewNum  int // line number in new file (0 if removed)
}

type DiffLineType int

const (
	DiffContext DiffLineType = iota
	DiffAdded
	DiffRemoved
)

// FileDiff represents the diff of a single file.
type FileDiff struct {
	Path    string
	OldPath string // empty if same as Path
	Status  string // "added", "removed", "modified"
	Lines   []DiffLine
}

// Diff computes a line-by-line diff between two strings using the Myers algorithm (simple LCS approach).
func Diff(oldText, newText string) []DiffLine {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	// Compute LCS table
	lcs := computeLCS(oldLines, newLines)

	// Backtrack to produce diff
	return backtrack(oldLines, newLines, lcs)
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// Remove trailing empty line from final newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// computeLCS builds the LCS length table.
func computeLCS(a, b []string) [][]int {
	m, n := len(a), len(b)
	table := make([][]int, m+1)
	for i := range table {
		table[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				table[i][j] = table[i-1][j-1] + 1
			} else if table[i-1][j] >= table[i][j-1] {
				table[i][j] = table[i-1][j]
			} else {
				table[i][j] = table[i][j-1]
			}
		}
	}
	return table
}

// backtrack produces diff lines from the LCS table.
func backtrack(oldLines, newLines []string, lcs [][]int) []DiffLine {
	var result []DiffLine
	i, j := len(oldLines), len(newLines)
	oldNum, newNum := len(oldLines), len(newLines)

	// Build in reverse, then flip
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			result = append(result, DiffLine{
				Type:    DiffContext,
				Content: oldLines[i-1],
				OldNum:  oldNum,
				NewNum:  newNum,
			})
			i--
			j--
			oldNum--
			newNum--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			result = append(result, DiffLine{
				Type:    DiffAdded,
				Content: newLines[j-1],
				NewNum:  newNum,
			})
			j--
			newNum--
		} else if i > 0 {
			result = append(result, DiffLine{
				Type:    DiffRemoved,
				Content: oldLines[i-1],
				OldNum:  oldNum,
			})
			i--
			oldNum--
		}
	}

	// Reverse
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

// FormatDiff formats a FileDiff as a unified-style colored string.
func FormatDiff(fd *FileDiff) string {
	var sb strings.Builder

	switch fd.Status {
	case "added":
		sb.WriteString(fmt.Sprintf("\033[1m--- /dev/null\033[0m\n"))
		sb.WriteString(fmt.Sprintf("\033[1m+++ %s\033[0m\n", fd.Path))
	case "removed":
		sb.WriteString(fmt.Sprintf("\033[1m--- %s\033[0m\n", fd.Path))
		sb.WriteString(fmt.Sprintf("\033[1m+++ /dev/null\033[0m\n"))
	default:
		old := fd.Path
		if fd.OldPath != "" {
			old = fd.OldPath
		}
		sb.WriteString(fmt.Sprintf("\033[1m--- %s\033[0m\n", old))
		sb.WriteString(fmt.Sprintf("\033[1m+++ %s\033[0m\n", fd.Path))
	}

	for _, line := range fd.Lines {
		switch line.Type {
		case DiffAdded:
			sb.WriteString(fmt.Sprintf("\033[32m+%s\033[0m\n", line.Content))
		case DiffRemoved:
			sb.WriteString(fmt.Sprintf("\033[31m-%s\033[0m\n", line.Content))
		case DiffContext:
			sb.WriteString(fmt.Sprintf(" %s\n", line.Content))
		}
	}
	return sb.String()
}
