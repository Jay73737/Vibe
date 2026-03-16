package core

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const VibeDirName = ".vibe"

// Repo represents a Vibe repository.
type Repo struct {
	WorkDir string       // working directory root
	VibeDir string       // path to .vibe
	Store   *ObjectStore // content-addressable store
}

// Index tracks staged files (path -> hash).
type Index struct {
	Entries map[string]Hash `json:"entries"`
}

// FindRepo walks up from the given directory to find a .vibe directory.
func FindRepo(startDir string) (*Repo, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}
	for {
		vibeDir := filepath.Join(dir, VibeDirName)
		if info, err := os.Stat(vibeDir); err == nil && info.IsDir() {
			return &Repo{
				WorkDir: dir,
				VibeDir: vibeDir,
				Store:   NewObjectStore(vibeDir),
			}, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("not a vibe repository (or any parent up to root)")
		}
		dir = parent
	}
}

// InitRepo creates a new Vibe repository in the given directory.
func InitRepo(dir string) (*Repo, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	vibeDir := filepath.Join(absDir, VibeDirName)

	if _, err := os.Stat(vibeDir); err == nil {
		return nil, fmt.Errorf("already a vibe repository: %s", absDir)
	}

	// Create directory structure
	dirs := []string{
		filepath.Join(vibeDir, "objects"),
		filepath.Join(vibeDir, "refs", "branches"),
		filepath.Join(vibeDir, "refs", "sessions"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", d, err)
		}
	}

	// Write HEAD pointing to main branch
	if err := os.WriteFile(filepath.Join(vibeDir, "HEAD"), []byte("ref: refs/branches/main\n"), 0644); err != nil {
		return nil, fmt.Errorf("write HEAD: %w", err)
	}

	// Write empty index
	emptyIndex := Index{Entries: make(map[string]Hash)}
	indexData, _ := json.Marshal(emptyIndex)
	if err := os.WriteFile(filepath.Join(vibeDir, "index"), indexData, 0644); err != nil {
		return nil, fmt.Errorf("write index: %w", err)
	}

	return &Repo{
		WorkDir: absDir,
		VibeDir: vibeDir,
		Store:   NewObjectStore(vibeDir),
	}, nil
}

// Head returns the current branch name and its commit hash.
func (r *Repo) Head() (branch string, commitHash Hash, err error) {
	data, err := os.ReadFile(filepath.Join(r.VibeDir, "HEAD"))
	if err != nil {
		return "", Hash{}, fmt.Errorf("read HEAD: %w", err)
	}
	ref := strings.TrimSpace(string(data))
	if !strings.HasPrefix(ref, "ref: ") {
		return "", Hash{}, fmt.Errorf("detached HEAD not yet supported")
	}
	branch = strings.TrimPrefix(ref, "ref: refs/branches/")
	refPath := filepath.Join(r.VibeDir, strings.TrimPrefix(ref, "ref: "))
	commitData, err := os.ReadFile(refPath)
	if err != nil {
		// Branch exists but has no commits yet
		return branch, Hash{}, nil
	}
	h, err := HashFromHex(strings.TrimSpace(string(commitData)))
	if err != nil {
		return branch, Hash{}, fmt.Errorf("parse ref hash: %w", err)
	}
	return branch, h, nil
}

// UpdateRef sets a branch ref to point at the given commit hash.
func (r *Repo) UpdateRef(branch string, h Hash) error {
	refPath := filepath.Join(r.VibeDir, "refs", "branches", branch)
	if err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(refPath, []byte(h.String()+"\n"), 0644)
}

// ReadIndex loads the staging index.
func (r *Repo) ReadIndex() (*Index, error) {
	data, err := os.ReadFile(filepath.Join(r.VibeDir, "index"))
	if err != nil {
		return &Index{Entries: make(map[string]Hash)}, nil
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}
	if idx.Entries == nil {
		idx.Entries = make(map[string]Hash)
	}
	return &idx, nil
}

// WriteIndex saves the staging index.
func (r *Repo) WriteIndex(idx *Index) error {
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.VibeDir, "index"), data, 0644)
}

// AddToIndex stages a file by hashing its content and adding it to the index.
func (r *Repo) AddToIndex(relPath string) error {
	absPath := filepath.Join(r.WorkDir, relPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read file %s: %w", relPath, err)
	}
	h, err := r.Store.WriteBlob(data)
	if err != nil {
		return fmt.Errorf("store blob for %s: %w", relPath, err)
	}
	idx, err := r.ReadIndex()
	if err != nil {
		return err
	}
	// Normalize path separators to forward slashes
	idx.Entries[filepath.ToSlash(relPath)] = h
	return r.WriteIndex(idx)
}

// CreateCommit creates a new commit from the current index.
func (r *Repo) CreateCommit(author, message string) (Hash, error) {
	idx, err := r.ReadIndex()
	if err != nil {
		return Hash{}, err
	}
	if len(idx.Entries) == 0 {
		return Hash{}, fmt.Errorf("nothing to commit (empty index)")
	}

	// Build tree from index
	tree := r.buildTreeFromIndex(idx)
	treeHash, err := r.Store.WriteTree(tree)
	if err != nil {
		return Hash{}, fmt.Errorf("write tree: %w", err)
	}

	// Get parent commit
	_, parentHash, err := r.Head()
	if err != nil {
		return Hash{}, fmt.Errorf("read HEAD: %w", err)
	}

	commit := &Commit{
		TreeHash:   treeHash,
		ParentHash: parentHash,
		Author:     author,
		Message:    message,
		Timestamp:  time.Now().UTC(),
	}
	commitHash, err := r.Store.WriteCommit(commit)
	if err != nil {
		return Hash{}, fmt.Errorf("write commit: %w", err)
	}

	// Update branch ref
	branch, _, _ := r.Head()
	if err := r.UpdateRef(branch, commitHash); err != nil {
		return Hash{}, fmt.Errorf("update ref: %w", err)
	}

	return commitHash, nil
}

// buildTreeFromIndex creates a flat tree from the index entries.
// For now, this produces a single-level tree; nested trees come later.
func (r *Repo) buildTreeFromIndex(idx *Index) *Tree {
	tree := &Tree{}
	for path, hash := range idx.Entries {
		tree.Entries = append(tree.Entries, TreeEntry{
			Name: path,
			Type: BlobObject,
			Hash: hash,
			Mode: 0644,
		})
	}
	return tree
}

// Status returns lists of staged, modified, and untracked files.
func (r *Repo) Status() (staged []string, modified []string, untracked []string, err error) {
	idx, err := r.ReadIndex()
	if err != nil {
		return nil, nil, nil, err
	}

	// Track which index entries we've seen in the working directory
	seen := make(map[string]bool)

	// Walk working directory
	err = filepath.WalkDir(r.WorkDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Skip .vibe directory
		if d.IsDir() && d.Name() == VibeDirName {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		relPath := filepath.ToSlash(strings.TrimPrefix(
			strings.TrimPrefix(path, r.WorkDir), string(filepath.Separator),
		))
		if relPath == "" {
			return nil
		}

		if hash, inIndex := idx.Entries[relPath]; inIndex {
			seen[relPath] = true
			// Check if modified since staging
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			currentBlob := append([]byte("blob\x00"), data...)
			currentHash := HashBytes(currentBlob)
			if currentHash != hash {
				modified = append(modified, relPath)
			} else {
				staged = append(staged, relPath)
			}
		} else {
			untracked = append(untracked, relPath)
		}
		return nil
	})

	return staged, modified, untracked, err
}
