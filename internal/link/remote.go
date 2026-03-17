package link

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/vibe-vcs/vibe/internal/core"
)

// RemoteClient handles HTTP communication with a Vibe server.
type RemoteClient struct {
	BaseURL string
	Token   string
}

func NewRemoteClient(url, token string) *RemoteClient {
	return &RemoteClient{
		BaseURL: strings.TrimRight(url, "/"),
		Token:   token,
	}
}

func (c *RemoteClient) get(path string) ([]byte, error) {
	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized — check your token")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d for %s", resp.StatusCode, path)
	}
	return io.ReadAll(resp.Body)
}

// ServerInfo holds the response from /api/info.
type ServerInfo struct {
	Branch    string   `json:"branch"`
	Head      string   `json:"head"`
	TunnelURL string   `json:"tunnel_url,omitempty"`
	ServerID  string   `json:"server_id,omitempty"`
	LANURLs   []string `json:"lan_urls,omitempty"`
	RelayURL   string   `json:"relay_url,omitempty"`
	RelayToken string   `json:"relay_token,omitempty"`
	Port      int      `json:"port,omitempty"`
}

// GetInfo returns server repo info including tunnel URL for re-discovery.
func (c *RemoteClient) GetInfo() (branch string, head string, err error) {
	info, err := c.GetServerInfo()
	if err != nil {
		return "", "", err
	}
	return info.Branch, info.Head, nil
}

// GetServerInfo returns the full server info including tunnel URL.
func (c *RemoteClient) GetServerInfo() (*ServerInfo, error) {
	data, err := c.get("/api/info")
	if err != nil {
		return nil, err
	}
	var info ServerInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// GetRefs returns all branch refs from the server.
func (c *RemoteClient) GetRefs() (map[string]string, error) {
	data, err := c.get("/api/refs")
	if err != nil {
		return nil, err
	}
	var result struct {
		Refs map[string]string `json:"refs"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result.Refs, nil
}

// GetManifest returns the file manifest from the server.
func (c *RemoteClient) GetManifest() (branch, head string, files map[string]ManifestEntry, err error) {
	data, err := c.get("/api/manifest")
	if err != nil {
		return "", "", nil, err
	}
	var result struct {
		Branch string                       `json:"branch"`
		Head   string                       `json:"head"`
		Files  map[string]ManifestEntry     `json:"files"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", "", nil, err
	}
	return result.Branch, result.Head, result.Files, nil
}

type ManifestEntry struct {
	Hash string `json:"hash"`
	Mode uint32 `json:"mode"`
}

// GetObject downloads a raw object by hash.
func (c *RemoteClient) GetObject(hashStr string) ([]byte, error) {
	return c.get("/api/objects/" + hashStr)
}

// GetBlob downloads blob content by hash.
func (c *RemoteClient) GetBlob(hashStr string) ([]byte, error) {
	return c.get("/api/blob/" + hashStr)
}

// LinkRemote connects a local directory to a remote Vibe server.
func LinkRemote(targetDir, serverURL, token string) (*core.Repo, error) {
	client := NewRemoteClient(serverURL, token)

	// Verify server is reachable and get full server info
	info, err := client.GetServerInfo()
	if err != nil {
		return nil, fmt.Errorf("cannot reach server: %w", err)
	}
	branch := info.Branch
	head := info.Head

	// Initialize target repo
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, err
	}
	os.MkdirAll(absTarget, 0755)

	repo, err := core.InitRepo(absTarget)
	if err != nil {
		repo, err = core.FindRepo(absTarget)
		if err != nil {
			return nil, err
		}
	}

	// Build fallback URLs: store every alternate way to reach this server
	// so the daemon can re-discover if the tunnel URL changes on restart.
	var fallbacks []string
	if info.TunnelURL != "" && info.TunnelURL != serverURL {
		fallbacks = append(fallbacks, info.TunnelURL)
	}
	// If linking via tunnel, keep the original URL as a fallback too
	if strings.Contains(serverURL, "trycloudflare.com") {
		fallbacks = append(fallbacks, serverURL)
	}
	// Store all LAN URLs the server reports — these are stable across restarts
	// and let the daemon reach the server to discover the new tunnel URL.
	for _, lanURL := range info.LANURLs {
		if lanURL != serverURL {
			fallbacks = append(fallbacks, lanURL)
		}
	}
	// Save link config
	config := LinkConfig{
		Source:       serverURL,
		SourceType:   "remote",
		Branch:       branch,
		Token:        token,
		FallbackURLs: fallbacks,
		RelayURL:     info.RelayURL,
		RelayToken:   info.RelayToken,
		ServerID:     info.ServerID,
	}
	if err := saveLinkConfig(repo, &config); err != nil {
		return nil, err
	}

	// Sync refs (HEAD will be set when working branch is created below)
	refs, err := client.GetRefs()
	if err == nil {
		for name, hashStr := range refs {
			h, err := core.HashFromHex(hashStr)
			if err != nil {
				continue
			}
			repo.UpdateRef(name, h)
		}
	}

	// Download objects for the head commit (commit + tree, not blobs yet)
	if head != "" {
		zeroHash := core.Hash{}.String()
		if head != zeroHash {
			// Fetch commit object
			commitData, err := client.GetObject(head)
			if err == nil {
				writeRawObject(repo, head, commitData)
			}

			// Parse commit to get tree hash
			h, _ := core.HashFromHex(head)
			commit, err := repo.Store.ReadCommit(h)
			if err == nil {
				treeHashStr := commit.TreeHash.String()
				treeData, err := client.GetObject(treeHashStr)
				if err == nil {
					writeRawObject(repo, treeHashStr, treeData)
				}
			}
		}
	}

	// Build manifest from server
	_, _, files, err := client.GetManifest()
	if err == nil && len(files) > 0 {
		manifest := &FileManifest{Files: make(map[string]FileInfo)}
		for name, entry := range files {
			h, _ := core.HashFromHex(entry.Hash)
			manifest.Files[name] = FileInfo{
				Hash:   h,
				Mode:   entry.Mode,
				Cached: false,
			}
			// Create directory structure
			dir := filepath.Dir(filepath.Join(repo.WorkDir, filepath.FromSlash(name)))
			os.MkdirAll(dir, 0755)
		}
		saveManifest(repo, manifest)
	}

	// Create a working branch so the user doesn't edit upstream directly
	headHash, _ := core.HashFromHex(head)
	if !headHash.IsZero() {
		workingBranch := createWorkingBranch(repo, branch, headHash)
		config.WorkingBranch = workingBranch
		saveLinkConfig(repo, &config)
	}

	return repo, nil
}

// FetchRemote fetches a file from the remote server.
func FetchRemote(repo *core.Repo, config *LinkConfig, relPath string, blobHash core.Hash) ([]byte, error) {
	client := NewRemoteClient(config.Source, config.Token)
	data, err := client.GetBlob(blobHash.String())
	if err != nil {
		return nil, fmt.Errorf("fetch from server: %w", err)
	}

	// Cache blob locally
	repo.Store.WriteBlob(data)

	// Write to working directory
	absPath := filepath.Join(repo.WorkDir, filepath.FromSlash(relPath))
	os.MkdirAll(filepath.Dir(absPath), 0755)
	if err := os.WriteFile(absPath, data, 0644); err != nil {
		return nil, err
	}

	return data, nil
}

// SyncRemote pulls latest changes from the remote server.
func SyncRemote(repo *core.Repo, config *LinkConfig) (int, error) {
	client := NewRemoteClient(config.Source, config.Token)

	// Get latest refs
	refs, err := client.GetRefs()
	if err != nil {
		return 0, fmt.Errorf("get refs: %w", err)
	}
	for name, hashStr := range refs {
		h, err := core.HashFromHex(hashStr)
		if err != nil {
			continue
		}
		repo.UpdateRef(name, h)

		// Fetch commit and tree objects
		if !repo.Store.HasObject(h) {
			commitData, err := client.GetObject(hashStr)
			if err == nil {
				writeRawObject(repo, hashStr, commitData)
			}
		}
		commit, err := repo.Store.ReadCommit(h)
		if err == nil && !repo.Store.HasObject(commit.TreeHash) {
			treeData, err := client.GetObject(commit.TreeHash.String())
			if err == nil {
				writeRawObject(repo, commit.TreeHash.String(), treeData)
			}
		}
	}

	// Update manifest
	_, _, files, err := client.GetManifest()
	if err != nil {
		return 0, err
	}

	oldManifest, _ := loadManifest(repo)
	newManifest := &FileManifest{Files: make(map[string]FileInfo)}
	changed := 0

	for name, entry := range files {
		h, _ := core.HashFromHex(entry.Hash)
		cached := false
		if old, exists := oldManifest.Files[name]; exists && old.Hash == h {
			cached = old.Cached
		} else {
			changed++
		}
		newManifest.Files[name] = FileInfo{
			Hash:   h,
			Mode:   entry.Mode,
			Cached: cached,
		}
	}

	saveManifest(repo, newManifest)
	return changed, nil
}

func writeRawObject(repo *core.Repo, hashStr string, data []byte) {
	objPath := filepath.Join(repo.VibeDir, "objects", hashStr[:2], hashStr[2:])
	os.MkdirAll(filepath.Dir(objPath), 0755)
	os.WriteFile(objPath, data, 0444)
}
