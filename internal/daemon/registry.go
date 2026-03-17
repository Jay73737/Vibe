package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Registry tracks which linked repos the daemon should watch.
// Stored at ~/.vibe/daemon.json.
type Registry struct {
	Repos []WatchedRepo `json:"repos"`
	mu    sync.Mutex
	path  string
}

// WatchedRepo represents a linked repo the daemon monitors.
type WatchedRepo struct {
	Path         string   `json:"path"`                    // absolute path to the repo working directory
	Source       string   `json:"source"`                  // remote server URL or local path
	SourceType   string   `json:"source_type"`             // "remote" or "local"
	Token        string   `json:"token,omitempty"`
	Branch       string   `json:"branch"`
	FallbackURLs []string `json:"fallback_urls,omitempty"` // alternate URLs to try if primary fails
	RelayURL     string   `json:"relay_url,omitempty"`     // relay server for URL discovery
	RelayToken   string   `json:"relay_token,omitempty"`   // per-repo token for relay auth
	ServerID     string   `json:"server_id,omitempty"`     // stable server identifier for relay lookups
}

// RegistryPath returns the default daemon registry path.
func RegistryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".vibe", "daemon.json")
}

// LoadRegistry loads the daemon registry from disk.
func LoadRegistry() (*Registry, error) {
	return LoadRegistryFrom(RegistryPath())
}

// LoadRegistryFrom loads the registry from a specific path.
func LoadRegistryFrom(path string) (*Registry, error) {
	r := &Registry{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, err
	}
	return r, nil
}

// Save writes the registry to disk.
func (r *Registry) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0644)
}

// Register adds a repo to the watch list (no duplicates by path).
func (r *Registry) Register(repo WatchedRepo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, existing := range r.Repos {
		if existing.Path == repo.Path {
			r.Repos[i] = repo // update in place
			return
		}
	}
	r.Repos = append(r.Repos, repo)
}

// Unregister removes a repo from the watch list.
func (r *Registry) Unregister(repoPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, existing := range r.Repos {
		if existing.Path == repoPath {
			r.Repos = append(r.Repos[:i], r.Repos[i+1:]...)
			return
		}
	}
}
