package branch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vibe-vcs/vibe/internal/core"
)

// Session represents an auto-saved snapshot of work when switching branches.
type Session struct {
	ID        string            `json:"id"`
	Branch    string            `json:"branch"`
	Timestamp time.Time         `json:"timestamp"`
	Message   string            `json:"message"`
	Index     map[string]core.Hash `json:"index"`
	// WorkingFiles stores the content hashes of modified (unstaged) working files
	WorkingFiles map[string]core.Hash `json:"working_files,omitempty"`
}

// Manager handles branch and session operations.
type Manager struct {
	Repo *core.Repo
}

func NewManager(repo *core.Repo) *Manager {
	return &Manager{Repo: repo}
}

// Create creates a new branch pointing at the current HEAD commit.
func (m *Manager) Create(name string) error {
	if err := validateBranchName(name); err != nil {
		return err
	}
	refPath := filepath.Join(m.Repo.VibeDir, "refs", "branches", name)
	if _, err := os.Stat(refPath); err == nil {
		return fmt.Errorf("branch '%s' already exists", name)
	}

	_, headHash, err := m.Repo.Head()
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	if headHash.IsZero() {
		return fmt.Errorf("cannot create branch: no commits yet (commit first)")
	}

	return m.Repo.UpdateRef(name, headHash)
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
		if err := m.saveSession(currentBranch); err != nil {
			return fmt.Errorf("save session: %w", err)
		}
	}

	// Read target branch commit hash
	targetData, err := os.ReadFile(refPath)
	if err != nil {
		return fmt.Errorf("read branch ref: %w", err)
	}
	targetHash, err := core.HashFromHex(strings.TrimSpace(string(targetData)))
	if err != nil {
		return fmt.Errorf("parse branch hash: %w", err)
	}

	// Checkout: restore working tree to match the target commit's tree
	if err := m.checkout(targetHash); err != nil {
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

	// Remove branch ref
	if err := os.Remove(refPath); err != nil {
		return fmt.Errorf("remove branch ref: %w", err)
	}

	// Remove all sessions for this branch
	sessionDir := filepath.Join(m.Repo.VibeDir, "refs", "sessions", name)
	os.RemoveAll(sessionDir)

	return nil
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
			return nil, nil // no sessions directory yet
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
	// Find the session
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

	// Restore the index
	idx := &core.Index{Entries: session.Index}
	if err := m.Repo.WriteIndex(idx); err != nil {
		return fmt.Errorf("restore index: %w", err)
	}

	// Restore working files from stored blobs
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
func (m *Manager) saveSession(branchName string) error {
	idx, err := m.Repo.ReadIndex()
	if err != nil {
		return err
	}

	// Capture modified working files (files that differ from the index)
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
		Message:      fmt.Sprintf("Auto-saved session on switch from %s", branchName),
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
	if strings.ContainsAny(name, " \t\n/\\..") {
		return fmt.Errorf("invalid branch name '%s' (no spaces, slashes, or dots allowed)", name)
	}
	return nil
}
