package branch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Jay73737/Vibe/internal/core"
)

// Session represents an auto-saved snapshot of work when switching branches.
type Session struct {
	ID           string               `json:"id"`
	Branch       string               `json:"branch"`
	Timestamp    time.Time            `json:"timestamp"`
	Message      string               `json:"message"`
	Index        map[string]core.Hash `json:"index"`
	WorkingFiles map[string]core.Hash `json:"working_files,omitempty"`
}

// Manager handles branch and session operations.
type Manager struct {
	Repo *core.Repo
}

func NewManager(repo *core.Repo) *Manager {
	return &Manager{Repo: repo}
}

// Create creates a new branch pointing at the current HEAD commit,
// recording the parent branch and fork point for future merges.
func (m *Manager) Create(name string) error {
	return m.CreateFrom(name, "", "")
}

// CreateFrom creates a new branch with explicit parent, author, and description.
// If parentBranch is empty, the current branch is used as the parent.
func (m *Manager) CreateFrom(name, author, description string) error {
	if err := validateBranchName(name); err != nil {
		return err
	}
	refPath := filepath.Join(m.Repo.VibeDir, "refs", "branches", name)
	if _, err := os.Stat(refPath); err == nil {
		return fmt.Errorf("branch '%s' already exists", name)
	}

	parentBranch, headHash, err := m.Repo.Head()
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	if headHash.IsZero() {
		return fmt.Errorf("cannot create branch: no commits yet (commit first)")
	}

	meta := &core.BranchMeta{
		Head:        headHash,
		Parent:      parentBranch,
		ForkPoint:   headHash,
		Author:      author,
		Description: description,
		CreatedAt:   time.Now().UTC(),
	}
	return m.Repo.WriteBranchMeta(name, meta)
}

// List returns all branch names and indicates which is current.
func (m *Manager) List() (branches []string, current string, err error) {
	current, _, err = m.Repo.Head()
	if err != nil {
		return nil, "", err
	}

	branchDir := filepath.Join(m.Repo.VibeDir, "refs", "branches")
	entries, err := os.ReadDir(branchDir)
	if err != nil {
		return nil, current, nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			branches = append(branches, e.Name())
		}
	}
	return branches, current, nil
}

// Switch changes the current branch. Auto-saves a session of the current work
// unless noSession is true.
func (m *Manager) Switch(target string, noSession bool) error {
	// Verify target branch exists
	refPath := filepath.Join(m.Repo.VibeDir, "refs", "branches", target)
	if _, err := os.Stat(refPath); err != nil {
		return fmt.Errorf("branch '%s' does not exist", target)
	}

	currentBranch, _, err := m.Repo.Head()
	if err != nil {
		return err
	}
	if currentBranch == target {
		return fmt.Errorf("already on branch '%s'", target)
	}

	// Auto-save session of current work
	if !noSession {
		desc := fmt.Sprintf("Work on '%s' before switching to '%s'", currentBranch, target)
		if err := m.saveSession(currentBranch, desc); err != nil {
			return fmt.Errorf("save session: %w", err)
		}
	}

	// Read target branch commit hash via metadata
	meta, err := m.Repo.ReadBranchMeta(target)
	if err != nil {
		return fmt.Errorf("read branch ref: %w", err)
	}

	// Checkout: restore working tree to match the target commit's tree
	if err := m.checkout(meta.Head); err != nil {
		return fmt.Errorf("checkout: %w", err)
	}

	// Update HEAD to point to target branch
	headPath := filepath.Join(m.Repo.VibeDir, "HEAD")
	return os.WriteFile(headPath, []byte("ref: refs/branches/"+target+"\n"), 0644)
}

// Destroy deletes a branch and all its sessions.
func (m *Manager) Destroy(name string) error {
	currentBranch, _, err := m.Repo.Head()
	if err != nil {
		return err
	}
	if currentBranch == name {
		return fmt.Errorf("cannot destroy the current branch '%s' (switch first)", name)
	}

	refPath := filepath.Join(m.Repo.VibeDir, "refs", "branches", name)
	if _, err := os.Stat(refPath); err != nil {
		return fmt.Errorf("branch '%s' does not exist", name)
	}

	if err := os.Remove(refPath); err != nil {
		return fmt.Errorf("remove branch ref: %w", err)
	}

	sessionDir := filepath.Join(m.Repo.VibeDir, "refs", "sessions", name)
	os.RemoveAll(sessionDir)
	return nil
}

// Merge performs a three-way merge of sourceBranch into the current branch.
// Returns the new commit hash and a list of conflicting files (kept current version).
func (m *Manager) Merge(sourceBranch, author string) (core.Hash, []string, error) {
	currentBranch, currentHead, err := m.Repo.Head()
	if err != nil {
		return core.Hash{}, nil, err
	}
	if currentHead.IsZero() {
		return core.Hash{}, nil, fmt.Errorf("current branch has no commits")
	}

	// Read source branch
	sourceMeta, err := m.Repo.ReadBranchMeta(sourceBranch)
	if err != nil {
		return core.Hash{}, nil, fmt.Errorf("branch '%s' does not exist", sourceBranch)
	}
	if sourceMeta.Head.IsZero() {
		return core.Hash{}, nil, fmt.Errorf("source branch '%s' has no commits", sourceBranch)
	}

	// Read current branch metadata for the fork point
	currentMeta, err := m.Repo.ReadBranchMeta(currentBranch)
	if err != nil {
		return core.Hash{}, nil, err
	}

	// Build file maps from trees
	sourceTree, err := m.treeForCommit(sourceMeta.Head)
	if err != nil {
		return core.Hash{}, nil, fmt.Errorf("read source tree: %w", err)
	}
	currentTree, err := m.treeForCommit(currentHead)
	if err != nil {
		return core.Hash{}, nil, fmt.Errorf("read current tree: %w", err)
	}

	// Try to get the base tree from fork point
	var baseFiles map[string]core.TreeEntry
	if !currentMeta.ForkPoint.IsZero() {
		baseTree, err := m.treeForCommit(currentMeta.ForkPoint)
		if err == nil {
			baseFiles = treeToMap(baseTree)
		}
	}

	sourceFiles := treeToMap(sourceTree)
	currentFiles := treeToMap(currentTree)

	// If no fork point, treat empty tree as base
	if baseFiles == nil {
		baseFiles = make(map[string]core.TreeEntry)
	}

	// Three-way merge
	merged := make(map[string]core.TreeEntry)
	var conflicts []string

	// Collect all filenames
	allFiles := make(map[string]bool)
	for name := range baseFiles {
		allFiles[name] = true
	}
	for name := range sourceFiles {
		allFiles[name] = true
	}
	for name := range currentFiles {
		allFiles[name] = true
	}

	for name := range allFiles {
		base, inBase := baseFiles[name]
		source, inSource := sourceFiles[name]
		current, inCurrent := currentFiles[name]

		switch {
		case inCurrent && inSource && inBase:
			// File exists in all three
			if current.Hash == base.Hash {
				// User didn't change it — take source version
				merged[name] = source
			} else if source.Hash == base.Hash {
				// Source didn't change it — keep user version
				merged[name] = current
			} else if current.Hash == source.Hash {
				// Both made the same change
				merged[name] = current
			} else {
				// Both sides diverged — keep current, flag conflict
				merged[name] = current
				conflicts = append(conflicts, name)
			}
		case inCurrent && inSource && !inBase:
			// File added in both — keep current if different
			if current.Hash == source.Hash {
				merged[name] = current
			} else {
				merged[name] = current
				conflicts = append(conflicts, name)
			}
		case inSource && !inCurrent && !inBase:
			// New file from source
			merged[name] = source
		case inCurrent && !inSource && !inBase:
			// New file from current
			merged[name] = current
		case inCurrent && !inSource && inBase:
			// Source deleted it, user still has it — keep user version
			merged[name] = current
		case !inCurrent && inSource && inBase:
			// User deleted it, source still has it — take source version
			merged[name] = source
		case inSource && inCurrent:
			// File in both (no base) — keep current
			if current.Hash != source.Hash {
				conflicts = append(conflicts, name)
			}
			merged[name] = current
		}
	}

	// Build merged tree
	mergedTree := &core.Tree{}
	for _, entry := range merged {
		mergedTree.Entries = append(mergedTree.Entries, entry)
	}
	treeHash, err := m.Repo.Store.WriteTree(mergedTree)
	if err != nil {
		return core.Hash{}, nil, fmt.Errorf("write merged tree: %w", err)
	}

	// Create merge commit
	conflictNote := ""
	if len(conflicts) > 0 {
		conflictNote = fmt.Sprintf(" (%d conflict(s) — kept your version)", len(conflicts))
	}
	commit := &core.Commit{
		TreeHash:    treeHash,
		ParentHash:  currentHead,
		MergeParent: sourceMeta.Head,
		Author:      author,
		Message:     fmt.Sprintf("Merge '%s' into '%s'%s", sourceBranch, currentBranch, conflictNote),
		Timestamp:   time.Now().UTC(),
	}
	commitHash, err := m.Repo.Store.WriteCommit(commit)
	if err != nil {
		return core.Hash{}, nil, fmt.Errorf("write merge commit: %w", err)
	}

	// Update current branch ref
	if err := m.Repo.UpdateRef(currentBranch, commitHash); err != nil {
		return core.Hash{}, nil, err
	}

	// Checkout the merged tree to working directory
	if err := m.checkout(commitHash); err != nil {
		return core.Hash{}, nil, fmt.Errorf("checkout merged tree: %w", err)
	}

	// Update fork point to the source head (so next merge knows the new base)
	currentMeta.ForkPoint = sourceMeta.Head
	currentMeta.Head = commitHash
	m.Repo.WriteBranchMeta(currentBranch, currentMeta)

	return commitHash, conflicts, nil
}

// treeForCommit reads the tree associated with a commit.
func (m *Manager) treeForCommit(commitHash core.Hash) (*core.Tree, error) {
	commit, err := m.Repo.Store.ReadCommit(commitHash)
	if err != nil {
		return nil, err
	}
	return m.Repo.Store.ReadTree(commit.TreeHash)
}

// treeToMap converts a tree's entries into a map keyed by filename.
func treeToMap(tree *core.Tree) map[string]core.TreeEntry {
	m := make(map[string]core.TreeEntry, len(tree.Entries))
	for _, e := range tree.Entries {
		m[e.Name] = e
	}
	return m
}

// Sessions returns all saved sessions, optionally filtered by branch.
func (m *Manager) Sessions(branch string) ([]Session, error) {
	sessionsRoot := filepath.Join(m.Repo.VibeDir, "refs", "sessions")
	var sessions []Session

	var dirs []string
	if branch != "" {
		dirs = []string{filepath.Join(sessionsRoot, branch)}
	} else {
		entries, err := os.ReadDir(sessionsRoot)
		if err != nil {
			return nil, nil
		}
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(sessionsRoot, e.Name()))
			}
		}
	}

	for _, dir := range dirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			data, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				continue
			}
			var s Session
			if err := json.Unmarshal(data, &s); err != nil {
				continue
			}
			sessions = append(sessions, s)
		}
	}
	return sessions, nil
}

// Restore restores a session by its ID, applying its index and working files.
func (m *Manager) Restore(sessionID string) error {
	sessionsRoot := filepath.Join(m.Repo.VibeDir, "refs", "sessions")
	var session *Session

	branchDirs, err := os.ReadDir(sessionsRoot)
	if err != nil {
		return fmt.Errorf("no sessions found")
	}
	for _, bd := range branchDirs {
		if !bd.IsDir() {
			continue
		}
		sessionPath := filepath.Join(sessionsRoot, bd.Name(), sessionID+".json")
		data, err := os.ReadFile(sessionPath)
		if err != nil {
			continue
		}
		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		session = &s
		break
	}

	if session == nil {
		return fmt.Errorf("session '%s' not found", sessionID)
	}

	idx := &core.Index{Entries: session.Index}
	if err := m.Repo.WriteIndex(idx); err != nil {
		return fmt.Errorf("restore index: %w", err)
	}

	for path, hash := range session.WorkingFiles {
		data, err := m.Repo.Store.ReadBlob(hash)
		if err != nil {
			return fmt.Errorf("restore file %s: %w", path, err)
		}
		absPath := filepath.Join(m.Repo.WorkDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(absPath, data, 0644); err != nil {
			return fmt.Errorf("write file %s: %w", path, err)
		}
	}

	return nil
}

// saveSession captures the current index and any modified working files.
func (m *Manager) saveSession(branchName, description string) error {
	idx, err := m.Repo.ReadIndex()
	if err != nil {
		return err
	}

	workingFiles := make(map[string]core.Hash)
	_, modified, _, err := m.Repo.Status()
	if err != nil {
		return err
	}
	for _, path := range modified {
		absPath := filepath.Join(m.Repo.WorkDir, filepath.FromSlash(path))
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		h, err := m.Repo.Store.WriteBlob(data)
		if err != nil {
			continue
		}
		workingFiles[path] = h
	}

	// Only save if there's something to save
	if len(idx.Entries) == 0 && len(workingFiles) == 0 {
		return nil
	}

	ts := time.Now().UTC()
	session := Session{
		ID:           fmt.Sprintf("%s-%d", branchName, ts.UnixMilli()),
		Branch:       branchName,
		Timestamp:    ts,
		Message:      description,
		Index:        idx.Entries,
		WorkingFiles: workingFiles,
	}

	sessionDir := filepath.Join(m.Repo.VibeDir, "refs", "sessions", branchName)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sessionDir, session.ID+".json"), data, 0644)
}

// checkout restores the working tree to match a commit's tree.
func (m *Manager) checkout(commitHash core.Hash) error {
	commit, err := m.Repo.Store.ReadCommit(commitHash)
	if err != nil {
		return err
	}
	tree, err := m.Repo.Store.ReadTree(commit.TreeHash)
	if err != nil {
		return err
	}

	// Clean current tracked files
	idx, _ := m.Repo.ReadIndex()
	for path := range idx.Entries {
		absPath := filepath.Join(m.Repo.WorkDir, filepath.FromSlash(path))
		os.Remove(absPath)
	}

	// Write files from the tree
	newIndex := &core.Index{Entries: make(map[string]core.Hash)}
	for _, entry := range tree.Entries {
		if entry.Type != core.BlobObject {
			continue
		}
		data, err := m.Repo.Store.ReadBlob(entry.Hash)
		if err != nil {
			return fmt.Errorf("read blob for %s: %w", entry.Name, err)
		}
		absPath := filepath.Join(m.Repo.WorkDir, filepath.FromSlash(entry.Name))
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(absPath, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", entry.Name, err)
		}
		newIndex.Entries[entry.Name] = entry.Hash
	}

	return m.Repo.WriteIndex(newIndex)
}

func validateBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("branch name cannot be empty")
	}
	if strings.ContainsAny(name, " \t\n/\\") {
		return fmt.Errorf("invalid branch name '%s' (no spaces or slashes allowed)", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid branch name '%s' (no '..' allowed)", name)
	}
	return nil
}
