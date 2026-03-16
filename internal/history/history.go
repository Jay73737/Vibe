package history

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vibe-vcs/vibe/internal/core"
)

// Manager handles version history operations.
type Manager struct {
	Repo *core.Repo
}

func NewManager(repo *core.Repo) *Manager {
	return &Manager{Repo: repo}
}

// DiffWorkingTree diffs the current working tree against the last commit (or index).
func (m *Manager) DiffWorkingTree() ([]FileDiff, error) {
	_, headHash, err := m.Repo.Head()
	if err != nil {
		return nil, err
	}

	// Get committed file contents
	committed := make(map[string]core.Hash)
	if !headHash.IsZero() {
		commit, err := m.Repo.Store.ReadCommit(headHash)
		if err != nil {
			return nil, err
		}
		tree, err := m.Repo.Store.ReadTree(commit.TreeHash)
		if err != nil {
			return nil, err
		}
		for _, entry := range tree.Entries {
			committed[entry.Name] = entry.Hash
		}
	}

	// Walk working directory
	working := make(map[string]string) // path -> content
	filepath.WalkDir(m.Repo.WorkDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == core.VibeDirName {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		relPath := filepath.ToSlash(strings.TrimPrefix(
			strings.TrimPrefix(path, m.Repo.WorkDir), string(filepath.Separator),
		))
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		working[relPath] = string(data)
		return nil
	})

	var diffs []FileDiff

	// Check for modified and removed files
	for path, hash := range committed {
		if content, exists := working[path]; exists {
			oldContent, err := m.Repo.Store.ReadBlob(hash)
			if err != nil {
				continue
			}
			if string(oldContent) != content {
				lines := Diff(string(oldContent), content)
				diffs = append(diffs, FileDiff{
					Path:   path,
					Status: "modified",
					Lines:  lines,
				})
			}
		} else {
			oldContent, _ := m.Repo.Store.ReadBlob(hash)
			lines := Diff(string(oldContent), "")
			diffs = append(diffs, FileDiff{
				Path:   path,
				Status: "removed",
				Lines:  lines,
			})
		}
	}

	// Check for added files
	for path, content := range working {
		if _, exists := committed[path]; !exists {
			lines := Diff("", content)
			diffs = append(diffs, FileDiff{
				Path:   path,
				Status: "added",
				Lines:  lines,
			})
		}
	}

	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Path < diffs[j].Path
	})
	return diffs, nil
}

// DiffCommits diffs two commits by their hashes.
func (m *Manager) DiffCommits(oldHash, newHash core.Hash) ([]FileDiff, error) {
	oldFiles, err := m.getCommitFiles(oldHash)
	if err != nil {
		return nil, fmt.Errorf("read old commit: %w", err)
	}
	newFiles, err := m.getCommitFiles(newHash)
	if err != nil {
		return nil, fmt.Errorf("read new commit: %w", err)
	}

	var diffs []FileDiff

	for path, oldHash := range oldFiles {
		oldContent, _ := m.Repo.Store.ReadBlob(oldHash)
		if newHash, exists := newFiles[path]; exists {
			newContent, _ := m.Repo.Store.ReadBlob(newHash)
			if string(oldContent) != string(newContent) {
				lines := Diff(string(oldContent), string(newContent))
				diffs = append(diffs, FileDiff{Path: path, Status: "modified", Lines: lines})
			}
		} else {
			lines := Diff(string(oldContent), "")
			diffs = append(diffs, FileDiff{Path: path, Status: "removed", Lines: lines})
		}
	}
	for path, newHash := range newFiles {
		if _, exists := oldFiles[path]; !exists {
			newContent, _ := m.Repo.Store.ReadBlob(newHash)
			lines := Diff("", string(newContent))
			diffs = append(diffs, FileDiff{Path: path, Status: "added", Lines: lines})
		}
	}

	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Path < diffs[j].Path
	})
	return diffs, nil
}

// Revert creates a new commit that restores the repo to the state of the given commit.
func (m *Manager) Revert(targetHash core.Hash, author string) (core.Hash, error) {
	// Read the target commit's tree
	targetCommit, err := m.Repo.Store.ReadCommit(targetHash)
	if err != nil {
		return core.Hash{}, fmt.Errorf("read target commit: %w", err)
	}
	targetTree, err := m.Repo.Store.ReadTree(targetCommit.TreeHash)
	if err != nil {
		return core.Hash{}, fmt.Errorf("read target tree: %w", err)
	}

	// Restore working directory to match target tree
	// First, clear all tracked files
	idx, _ := m.Repo.ReadIndex()
	for path := range idx.Entries {
		absPath := filepath.Join(m.Repo.WorkDir, filepath.FromSlash(path))
		os.Remove(absPath)
	}

	// Write target tree files and build new index
	newIndex := &core.Index{Entries: make(map[string]core.Hash)}
	for _, entry := range targetTree.Entries {
		if entry.Type != core.BlobObject {
			continue
		}
		data, err := m.Repo.Store.ReadBlob(entry.Hash)
		if err != nil {
			return core.Hash{}, fmt.Errorf("read blob %s: %w", entry.Name, err)
		}
		absPath := filepath.Join(m.Repo.WorkDir, filepath.FromSlash(entry.Name))
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return core.Hash{}, err
		}
		if err := os.WriteFile(absPath, data, 0644); err != nil {
			return core.Hash{}, err
		}
		newIndex.Entries[entry.Name] = entry.Hash
	}

	if err := m.Repo.WriteIndex(newIndex); err != nil {
		return core.Hash{}, err
	}

	// Create a revert commit
	message := fmt.Sprintf("Revert to %s", targetHash.Short())
	return m.Repo.CreateCommit(author, message)
}

// Blame returns per-line authorship information for a file.
func (m *Manager) Blame(filePath string) ([]BlameLine, error) {
	// Normalize path
	filePath = filepath.ToSlash(filePath)

	// Walk commit history, tracking when each line last changed
	_, headHash, err := m.Repo.Head()
	if err != nil || headHash.IsZero() {
		return nil, fmt.Errorf("no commits")
	}

	// Collect commit chain
	var commits []commitInfo
	h := headHash
	for !h.IsZero() {
		c, err := m.Repo.Store.ReadCommit(h)
		if err != nil {
			break
		}
		commits = append(commits, commitInfo{hash: h, commit: c})
		h = c.ParentHash
	}

	// Get current file content
	currentContent, err := m.getFileAtCommit(commits[0].hash, filePath)
	if err != nil {
		return nil, fmt.Errorf("file '%s' not found in HEAD", filePath)
	}
	currentLines := splitLines(currentContent)

	// Initialize blame: every line attributed to HEAD
	blame := make([]BlameLine, len(currentLines))
	for i, line := range currentLines {
		blame[i] = BlameLine{
			LineNum:    i + 1,
			Content:    line,
			CommitHash: commits[0].hash,
			Author:     commits[0].commit.Author,
			Timestamp:  commits[0].commit.Timestamp,
		}
	}

	// Walk backwards through commits and attribute lines to the earliest commit where they appeared
	for ci := 0; ci < len(commits)-1; ci++ {
		parentContent, err := m.getFileAtCommit(commits[ci+1].hash, filePath)
		if err != nil {
			// File didn't exist in parent — all remaining lines belong to this commit
			break
		}

		parentLines := splitLines(parentContent)
		childContent, _ := m.getFileAtCommit(commits[ci].hash, filePath)
		childLines := splitLines(childContent)

		// Find which lines are unchanged from parent
		diffLines := Diff(string(parentContent)+"\n", string(childContent)+"\n")
		parentLineMap := mapUnchangedLines(diffLines, parentLines, childLines)

		// For lines that exist unchanged in the parent, attribute to an older commit
		for childIdx, parentIdx := range parentLineMap {
			// Find the corresponding line in current blame
			if childIdx < len(blame) && parentIdx < len(parentLines) {
				// This line existed in the parent, so attribute to at least the parent's commit
				blame[childIdx].CommitHash = commits[ci+1].hash
				blame[childIdx].Author = commits[ci+1].commit.Author
				blame[childIdx].Timestamp = commits[ci+1].commit.Timestamp
			}
		}
	}

	return blame, nil
}

// BlameLine represents authorship info for a single line.
type BlameLine struct {
	LineNum    int
	Content    string
	CommitHash core.Hash
	Author     string
	Timestamp  interface{} // time.Time
}

type commitInfo struct {
	hash   core.Hash
	commit *core.Commit
}

func (m *Manager) getCommitFiles(h core.Hash) (map[string]core.Hash, error) {
	commit, err := m.Repo.Store.ReadCommit(h)
	if err != nil {
		return nil, err
	}
	tree, err := m.Repo.Store.ReadTree(commit.TreeHash)
	if err != nil {
		return nil, err
	}
	files := make(map[string]core.Hash)
	for _, entry := range tree.Entries {
		files[entry.Name] = entry.Hash
	}
	return files, nil
}

func (m *Manager) getFileAtCommit(h core.Hash, path string) (string, error) {
	files, err := m.getCommitFiles(h)
	if err != nil {
		return "", err
	}
	blobHash, exists := files[path]
	if !exists {
		return "", fmt.Errorf("file not found")
	}
	data, err := m.Repo.Store.ReadBlob(blobHash)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// mapUnchangedLines returns a map of childLineIndex -> parentLineIndex for context (unchanged) lines.
func mapUnchangedLines(diffLines []DiffLine, parentLines, childLines []string) map[int]int {
	result := make(map[int]int)
	for _, dl := range diffLines {
		if dl.Type == DiffContext && dl.NewNum > 0 && dl.OldNum > 0 {
			childIdx := dl.NewNum - 1
			parentIdx := dl.OldNum - 1
			if childIdx < len(childLines) && parentIdx < len(parentLines) {
				result[childIdx] = parentIdx
			}
		}
	}
	return result
}
