package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/Jay73737/Vibe/internal/core"
	"github.com/Jay73737/Vibe/internal/link"
	vibeRelay "github.com/Jay73737/Vibe/internal/relay"
)

// Daemon is the background service that watches linked repos and auto-syncs.
type Daemon struct {
	registry  *Registry
	watchers  map[string]*repoWatcher // keyed by repo path
	mu        sync.Mutex
	pollInterval time.Duration
	stopCh    chan struct{}
}

// repoWatcher monitors a single linked repo.
type repoWatcher struct {
	repo     WatchedRepo
	wsConn   *websocket.Conn
	stopCh   chan struct{}
	daemon   *Daemon
}

// wsEvent matches the server's Event struct.
type wsEvent struct {
	Type    string      `json:"type"`
	Branch  string      `json:"branch,omitempty"`
	Hash    string      `json:"hash,omitempty"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// New creates a new daemon instance.
func New() (*Daemon, error) {
	reg, err := LoadRegistry()
	if err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}
	return &Daemon{
		registry:     reg,
		watchers:     make(map[string]*repoWatcher),
		pollInterval: 30 * time.Second,
		stopCh:       make(chan struct{}),
	}, nil
}

// Run starts the daemon. Blocks until interrupted.
func (d *Daemon) Run() error {
	log.Println("vibe daemon starting...")

	// Reload registry to pick up any new repos
	reg, err := LoadRegistry()
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}
	d.registry = reg

	if len(d.registry.Repos) == 0 {
		log.Println("No repos registered. Link a repo first: vibe link <source>")
		log.Println("Waiting for repos to be registered...")
	} else {
		log.Printf("Watching %d repo(s)", len(d.registry.Repos))
	}

	// Start watchers for all registered repos
	for _, repo := range d.registry.Repos {
		d.startWatcher(repo)
	}

	// Periodic registry reload + poll loop
	go d.pollLoop()

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("Received %v, shutting down...", sig)
	case <-d.stopCh:
		log.Println("Daemon stopped.")
	}

	d.stopAll()
	return nil
}

// Stop signals the daemon to shut down.
func (d *Daemon) Stop() {
	close(d.stopCh)
}

func (d *Daemon) startWatcher(repo WatchedRepo) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.watchers[repo.Path]; exists {
		return // already watching
	}

	w := &repoWatcher{
		repo:   repo,
		stopCh: make(chan struct{}),
		daemon: d,
	}
	d.watchers[repo.Path] = w

	log.Printf("  Watching: %s (source: %s)", repo.Path, repo.Source)

	if repo.SourceType == "remote" {
		go w.watchRemote()
	} else {
		go w.watchLocal()
	}
}

func (d *Daemon) stopAll() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for path, w := range d.watchers {
		close(w.stopCh)
		if w.wsConn != nil {
			w.wsConn.Close()
		}
		delete(d.watchers, path)
	}
}

// pollLoop periodically reloads the registry and syncs repos as a fallback.
func (d *Daemon) pollLoop() {
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.reloadAndSync()
		case <-d.stopCh:
			return
		}
	}
}

func (d *Daemon) reloadAndSync() {
	// Reload registry to pick up newly linked repos
	reg, err := LoadRegistry()
	if err != nil {
		log.Printf("registry reload error: %v", err)
		return
	}

	// Start watchers for any new repos
	for _, repo := range reg.Repos {
		d.startWatcher(repo)
	}

	// Periodic sync for all repos (fallback in case WebSocket missed something)
	d.mu.Lock()
	watchers := make([]*repoWatcher, 0, len(d.watchers))
	for _, w := range d.watchers {
		watchers = append(watchers, w)
	}
	d.mu.Unlock()

	for _, w := range watchers {
		w.syncAndPull()
	}
}

// watchRemote connects to the remote server's WebSocket and auto-syncs on push events.
// If the primary URL fails, it tries fallback URLs and auto-discovers new tunnel URLs.
func (w *repoWatcher) watchRemote() {
	for {
		select {
		case <-w.stopCh:
			return
		default:
		}

		err := w.connectAndListen()
		if err != nil {
			log.Printf("[%s] WebSocket error: %v", w.repo.Path, err)

			// Primary URL failed — try fallback URLs to discover the new tunnel URL
			if w.tryFallbackDiscovery() {
				log.Printf("[%s] Discovered new server URL: %s", w.repo.Path, w.repo.Source)
				continue // retry immediately with new URL
			}

			log.Printf("[%s] All URLs failed, retrying in 30s...", w.repo.Path)
		}

		// Wait before reconnecting
		select {
		case <-w.stopCh:
			return
		case <-time.After(30 * time.Second):
		}
	}
}

// tryFallbackDiscovery attempts each fallback URL to find the server and
// discover its current tunnel URL. As a last resort, queries the relay server.
// Returns true if a working URL was found.
func (w *repoWatcher) tryFallbackDiscovery() bool {
	// Try each fallback URL (LAN IPs, old tunnel URLs)
	for _, fallbackURL := range w.repo.FallbackURLs {
		if fallbackURL == w.repo.Source {
			continue // already tried this one
		}

		log.Printf("[%s] Trying fallback URL: %s", w.repo.Path, fallbackURL)
		client := link.NewRemoteClient(fallbackURL, w.repo.Token)
		info, err := client.GetServerInfo()
		if err != nil {
			continue
		}

		// Server is reachable at this fallback! Check if it reports a tunnel URL.
		newURL := fallbackURL
		if info.TunnelURL != "" {
			newURL = info.TunnelURL
		}

		if newURL != w.repo.Source {
			w.updateSourceURL(newURL)
			return true
		}
		// Fallback itself works, use it
		return true
	}

	// Last resort: query the relay server
	if w.repo.RelayURL != "" && w.repo.ServerID != "" {
		log.Printf("[%s] Querying relay %s for server %s...", w.repo.Path, w.repo.RelayURL, w.repo.ServerID)
		entry, err := vibeRelay.Discover(w.repo.RelayURL, w.repo.ServerID, w.repo.RelayToken)
		if err == nil && entry.TunnelURL != "" && entry.TunnelURL != w.repo.Source {
			log.Printf("[%s] Relay returned new URL: %s", w.repo.Path, entry.TunnelURL)
			w.updateSourceURL(entry.TunnelURL)
			return true
		}
		if err != nil {
			log.Printf("[%s] Relay query failed: %v", w.repo.Path, err)
		}
	}

	return false
}

// updateSourceURL updates the primary source URL in both the daemon registry
// and the repo's link.json, keeping the old URL as a fallback.
func (w *repoWatcher) updateSourceURL(newURL string) {
	oldURL := w.repo.Source

	// Add old URL to fallbacks if not already there
	hasFallback := false
	for _, u := range w.repo.FallbackURLs {
		if u == oldURL {
			hasFallback = true
			break
		}
	}
	if !hasFallback {
		w.repo.FallbackURLs = append(w.repo.FallbackURLs, oldURL)
	}

	w.repo.Source = newURL

	// Update daemon registry
	reg, err := LoadRegistry()
	if err == nil {
		reg.Register(w.repo)
		reg.Save()
	}

	// Update the repo's link.json
	repo, err := core.FindRepo(w.repo.Path)
	if err == nil {
		cfg, err := link.LoadLinkConfig(repo)
		if err == nil {
			cfg.Source = newURL
			// Keep old URL in fallbacks
			hasOld := false
			for _, u := range cfg.FallbackURLs {
				if u == oldURL {
					hasOld = true
					break
				}
			}
			if !hasOld {
				cfg.FallbackURLs = append(cfg.FallbackURLs, oldURL)
			}
			link.SaveLinkConfig(repo, cfg)
		}
	}

	log.Printf("[%s] Updated source URL: %s -> %s", w.repo.Path, oldURL, newURL)
}

func (w *repoWatcher) connectAndListen() error {
	wsURL, err := httpToWS(w.repo.Source)
	if err != nil {
		return fmt.Errorf("build ws url: %w", err)
	}

	header := http.Header{}
	if w.repo.Token != "" {
		header.Set("Authorization", "Bearer "+w.repo.Token)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}
	w.wsConn = conn
	defer func() {
		conn.Close()
		w.wsConn = nil
	}()

	log.Printf("[%s] Connected to %s", w.repo.Path, wsURL)

	for {
		select {
		case <-w.stopCh:
			return nil
		default:
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var event wsEvent
		if err := json.Unmarshal(message, &event); err != nil {
			continue
		}

		switch event.Type {
		case "commit", "sync", "branch":
			log.Printf("[%s] Received %s event on branch %s — auto-syncing...", w.repo.Path, event.Type, event.Branch)
			w.syncAndPull()

		case "connected":
			log.Printf("[%s] Server says: %s", w.repo.Path, event.Message)
			// Check if server sent a tunnel URL we should know about
			w.handleTunnelData(event.Data)
			// Do an initial sync on connect
			w.syncAndPull()

		case "tunnel_update":
			log.Printf("[%s] Server tunnel URL changed!", w.repo.Path)
			w.handleTunnelData(event.Data)
		}
	}
}

// handleTunnelData extracts a tunnel_url from event data and updates our source if needed.
func (w *repoWatcher) handleTunnelData(data interface{}) {
	if data == nil {
		return
	}
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return
	}
	tunnelURL, ok := dataMap["tunnel_url"].(string)
	if !ok || tunnelURL == "" {
		return
	}

	// If our current source is a tunnel URL and it's different, update it
	if strings.Contains(w.repo.Source, "trycloudflare.com") && tunnelURL != w.repo.Source {
		log.Printf("[%s] Updating tunnel URL: %s -> %s", w.repo.Path, w.repo.Source, tunnelURL)
		w.updateSourceURL(tunnelURL)
	}
}

// watchLocal periodically syncs a locally-linked repo.
func (w *repoWatcher) watchLocal() {
	// For local repos, just poll periodically (the main poll loop handles this)
	// But do an initial sync
	w.syncAndPull()

	<-w.stopCh
}

// syncAndPull runs sync + pull for the watched repo.
func (w *repoWatcher) syncAndPull() {
	repo, err := core.FindRepo(w.repo.Path)
	if err != nil {
		log.Printf("[%s] repo error: %v", w.repo.Path, err)
		return
	}

	mgr := link.NewManager(repo)
	changed, err := mgr.Sync()
	if err != nil {
		log.Printf("[%s] sync error: %v", w.repo.Path, err)
		return
	}

	if changed > 0 {
		log.Printf("[%s] Synced %d changed file(s), pulling...", w.repo.Path, changed)
		count, err := mgr.Pull()
		if err != nil {
			log.Printf("[%s] pull error: %v", w.repo.Path, err)
			return
		}
		if count > 0 {
			log.Printf("[%s] Pulled %d file(s)", w.repo.Path, count)
		}
	}
}

// httpToWS converts an HTTP URL to a WebSocket URL.
func httpToWS(httpURL string) (string, error) {
	u, err := url.Parse(httpURL)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = "/ws"
	if u.Port() == "" {
		u.Host = u.Hostname() + ":7433"
	}
	return u.String(), nil
}

// RegisterRepo adds a repo to the daemon registry (called by vibe link).
func RegisterRepo(repoPath, source, sourceType, token, branch string, fallbackURLs []string, relayURL, relayToken, serverID string) error {
	reg, err := LoadRegistry()
	if err != nil {
		return err
	}
	absPath, err := absRepoPath(repoPath)
	if err != nil {
		return err
	}
	reg.Register(WatchedRepo{
		Path:         absPath,
		Source:       source,
		SourceType:   sourceType,
		Token:        token,
		Branch:       branch,
		FallbackURLs: fallbackURLs,
		RelayURL:     relayURL,
		RelayToken:   relayToken,
		ServerID:     serverID,
	})
	return reg.Save()
}

// UnregisterRepo removes a repo from the daemon registry.
func UnregisterRepo(repoPath string) error {
	reg, err := LoadRegistry()
	if err != nil {
		return err
	}
	absPath, err := absRepoPath(repoPath)
	if err != nil {
		return err
	}
	reg.Unregister(absPath)
	return reg.Save()
}

func absRepoPath(path string) (string, error) {
	if strings.HasPrefix(path, "/") || strings.Contains(path, ":\\") || strings.Contains(path, ":/") {
		return path, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return wd, nil
}
