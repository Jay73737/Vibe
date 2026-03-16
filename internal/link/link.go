package link

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/vibe-vcs/vibe/internal/core"
)

// LinkConfig stores the connection between a linked repo and its source.
type LinkConfig struct {
	Source     string `json:"source"`      // local path or URL
	SourceType string `json:"source_type"` // "local" or "remote"
	Branch     string `json:"branch"`      // branch to track
}

// Manager handles repo linking and syncing.
type Manager struct {
	Repo *core.Repo
}

func NewManager(repo *core.Repo) *Manager {
	return &Manager{Repo: repo}
}

// Link connects the current repo to a source repo.
// For local sources, it syncs metadata and directory structure immediately,
// with file contents fetched on-demand.
func Link(targetDir, source string) (*core.Repo, error) {
	sourceType := "local"
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		sourceType = "remote"
	}

	if sourceType == "remote" {
		return nil, fmt.Errorf("remote linking will be available in Phase 5 (server)")
	}

	// Verify source is a vibe repo
	sourceRepo, err := core.FindRepo(source)
	if err != nil {
		return nil, fmt.Errorf("source is not a vibe repository: %w", err)
	}

	// Initialize target as a new vibe repo
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absTarget, 0755); err != nil {
		return nil, err
	}

	repo, err := core.InitRepo(absTarget)
	if err != nil {
		// If already initialized, find existing
		repo, err = core.FindRepo(absTarget)
		if err != nil {
			return nil, err
		}
	}

	// Save link config
	config := LinkConfig{
		Source:     sourceRepo.WorkDir,
		SourceType: sourceType,
		Branch:     "main",
	}
	if err := saveLinkConfig(repo, &config); err != nil {
		return nil, fmt.Errorf("save link config: %w", err)
	}

	// Sync: copy all objects from source
	if err := syncObjects(sourceRepo, repo); err != nil {
		return nil, fmt.Errorf("sync objects: %w", err)
	}

	// Copy refs
	if err := syncRefs(sourceRepo, repo); err != nil {
		return nil, fmt.Errorf("sync refs: %w", err)
	}

	// Build directory structure with stub files (hybrid mode)
	// Metadata is synced, but file contents are fetched on-demand
	sourceBranch, sourceHead, err := sourceRepo.Head()
	if err != nil || sourceHead.IsZero() {
		// Empty repo, just link config is enough
		return repo, nil
	}

	// Point our HEAD to the same branch
	headPath := filepath.Join(repo.VibeDir, "HEAD")
	os.WriteFile(headPath, []byte("ref: refs/branches/"+sourceBranch+"\n"), 0644)

	// Create manifest of files (directory structure) without writing contents
	commit, err := sourceRepo.Store.ReadCommit(sourceHead)
	if err != nil {
		return repo, nil
	}
	tree, err := sourceRepo.Store.ReadTree(commit.TreeHash)
	if err != nil {
		return repo, nil
	}

	manifest := &FileManifest{Files: make(map[string]FileInfo)}
	for _, entry := range tree.Entries {
		manifest.Files[entry.Name] = FileInfo{
			Hash:   entry.Hash,
			Mode:   entry.Mode,
			Cached: false,
		}
	}
	if err := saveManifest(repo, manifest); err != nil {
		return nil, fmt.Errorf("save manifest: %w", err)
	}

	// Create empty placeholder directories so users can see the structure
	for name := range manifest.Files {
		dir := filepath.Dir(filepath.Join(repo.WorkDir, filepath.FromSlash(name)))
		os.MkdirAll(dir, 0755)
	}

	return repo, nil
}

// Fetch retrieves a specific file's content from the source, caching it locally.
func (m *Manager) Fetch(relPath string) ([]byte, error) {
	relPath = filepath.ToSlash(relPath)

	config, err := loadLinkConfig(m.Repo)
	if err != nil {
		return nil, fmt.Errorf("not a linked repo: %w", err)
	}

	manifest, err := loadManifest(m.Repo)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	info, exists := manifest.Files[relPath]
	if !exists {
		return nil, fmt.Errorf("file '%s' not in manifest", relPath)
	}

	// Check if already cached locally
	if info.Cached {
		data, err := m.Repo.Store.ReadBlob(info.Hash)
		if err == nil {
			return data, nil
		}
	}

	// Fetch from source
	if config.SourceType == "local" {
		sourceRepo, err := core.FindRepo(config.Source)
		if err != nil {
			return nil, fmt.Errorf("source repo unavailable: %w", err)
		}
		data, err := sourceRepo.Store.ReadBlob(info.Hash)
		if err != nil {
			return nil, fmt.Errorf("fetch blob from source: %w", err)
		}

		// Cache locally
		m.Repo.Store.WriteBlob(data)

		// Write to working directory
		absPath := filepath.Join(m.Repo.WorkDir, filepath.FromSlash(relPath))
		os.MkdirAll(filepath.Dir(absPath), 0755)
		if err := os.WriteFile(absPath, data, 0644); err != nil {
			return nil, fmt.Errorf("write file: %w", err)
		}

		// Update manifest
		info.Cached = true
		manifest.Files[relPath] = info
		saveManifest(m.Repo, manifest)

		// Update index
		m.Repo.AddToIndex(relPath)

		return data, nil
	}

	return nil, fmt.Errorf("remote fetch not yet supported")
}

// Pull fetches ALL files from the source at once.
func (m *Manager) Pull() (int, error) {
	manifest, err := loadManifest(m.Repo)
	if err != nil {
		return 0, fmt.Errorf("load manifest: %w", err)
	}

	count := 0
	for path, info := range manifest.Files {
		if info.Cached {
			continue
		}
		if _, err := m.Fetch(path); err != nil {
			return count, fmt.Errorf("fetch %s: %w", path, err)
		}
		count++
	}
	return count, nil
}

// Sync pulls the latest changes from the source repo.
func (m *Manager) Sync() (int, error) {
	config, err := loadLinkConfig(m.Repo)
	if err != nil {
		return 0, fmt.Errorf("not a linked repo: %w", err)
	}

	if config.SourceType != "local" {
		return 0, fmt.Errorf("remote sync not yet supported")
	}

	sourceRepo, err := core.FindRepo(config.Source)
	if err != nil {
		return 0, fmt.Errorf("source repo unavailable: %w", err)
	}

	// Sync new objects
	if err := syncObjects(sourceRepo, m.Repo); err != nil {
		return 0, fmt.Errorf("sync objects: %w", err)
	}

	// Sync refs
	if err := syncRefs(sourceRepo, m.Repo); err != nil {
		return 0, fmt.Errorf("sync refs: %w", err)
	}

	// Update manifest with new/changed files
	_, sourceHead, err := sourceRepo.Head()
	if err != nil || sourceHead.IsZero() {
		return 0, nil
	}

	commit, err := sourceRepo.Store.ReadCommit(sourceHead)
	if err != nil {
		return 0, err
	}
	tree, err := sourceRepo.Store.ReadTree(commit.TreeHash)
	if err != nil {
		return 0, err
	}

	oldManifest, _ := loadManifest(m.Repo)
	newManifest := &FileManifest{Files: make(map[string]FileInfo)}
	changed := 0

	for _, entry := range tree.Entries {
		cached := false
		if old, exists := oldManifest.Files[entry.Name]; exists && old.Hash == entry.Hash {
			cached = old.Cached
		} else {
			changed++
		}
		newManifest.Files[entry.Name] = FileInfo{
			Hash:   entry.Hash,
			Mode:   entry.Mode,
			Cached: cached,
		}
	}

	saveManifest(m.Repo, newManifest)
	return changed, nil
}

// Status returns the link status info.
func (m *Manager) Status() (*LinkConfig, *FileManifest, error) {
	config, err := loadLinkConfig(m.Repo)
	if err != nil {
		return nil, nil, err
	}
	manifest, err := loadManifest(m.Repo)
	if err != nil {
		return config, nil, nil
	}
	return config, manifest, nil
}

// FileManifest tracks which files exist in the linked repo and their cache status.
type FileManifest struct {
	Files map[string]FileInfo `json:"files"`
}

type FileInfo struct {
	Hash   core.Hash `json:"hash"`
	Mode   uint32    `json:"mode"`
	Cached bool      `json:"cached"`
}

func saveLinkConfig(repo *core.Repo, config *LinkConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(repo.VibeDir, "link.json"), data, 0644)
}

func loadLinkConfig(repo *core.Repo) (*LinkConfig, error) {
	data, err := os.ReadFile(filepath.Join(repo.VibeDir, "link.json"))
	if err != nil {
		return nil, err
	}
	var config LinkConfig
	return &config, json.Unmarshal(data, &config)
}

func saveManifest(repo *core.Repo, manifest *FileManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(repo.VibeDir, "manifest.json"), data, 0644)
}

func loadManifest(repo *core.Repo) (*FileManifest, error) {
	data, err := os.ReadFile(filepath.Join(repo.VibeDir, "manifest.json"))
	if err != nil {
		return &FileManifest{Files: make(map[string]FileInfo)}, nil
	}
	var manifest FileManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// syncObjects copies all objects from source to target that target doesn't have.
func syncObjects(source, target *core.Repo) error {
	objDir := filepath.Join(source.VibeDir, "objects")
	return filepath.WalkDir(objDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		// Reconstruct hash from path: objects/ab/cdef... -> abcdef...
		rel, _ := filepath.Rel(objDir, path)
		rel = filepath.ToSlash(rel)
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) != 2 {
			return nil
		}
		hexStr := parts[0] + parts[1]
		h, err := core.HashFromHex(hexStr)
		if err != nil {
			return nil
		}
		if target.Store.HasObject(h) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		// Write raw object to target
		targetPath := filepath.Join(target.VibeDir, "objects", hexStr[:2], hexStr[2:])
		os.MkdirAll(filepath.Dir(targetPath), 0755)
		return os.WriteFile(targetPath, data, 0444)
	})
}

// syncRefs copies branch refs from source to target.
func syncRefs(source, target *core.Repo) error {
	refsDir := filepath.Join(source.VibeDir, "refs", "branches")
	entries, err := os.ReadDir(refsDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(refsDir, e.Name()))
		if err != nil {
			continue
		}
		targetRefDir := filepath.Join(target.VibeDir, "refs", "branches")
		os.MkdirAll(targetRefDir, 0755)
		os.WriteFile(filepath.Join(targetRefDir, e.Name()), data, 0644)
	}
	return nil
}
