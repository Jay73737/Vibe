package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Relay is a lightweight URL discovery server.
// Vibe servers publish their current tunnel URL here keyed by repo ID.
// Vibe daemons query here when they can't reach their server's last known URL.
//
// The relay can be self-hosted anywhere with a stable address — it doesn't need
// cloudflared or any tunnel itself. It just needs to be reachable by clients.
type Relay struct {
	entries map[string]*Entry
	mu      sync.RWMutex
	dataDir string // persistent storage directory (empty = in-memory only)
}

// Entry is a single server registration.
type Entry struct {
	ServerID  string    `json:"server_id"`
	TunnelURL string    `json:"tunnel_url"`
	LANURLs   []string  `json:"lan_urls,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	Token     string    `json:"-"` // publish token, not exposed in reads
}

// PublishRequest is the body sent by vibe serve --relay.
type PublishRequest struct {
	ServerID  string   `json:"server_id"`
	TunnelURL string   `json:"tunnel_url"`
	LANURLs   []string `json:"lan_urls,omitempty"`
	Token     string   `json:"token"` // must match the relay's configured token
}

// New creates a relay server. If dataDir is non-empty, entries persist to disk.
func New(dataDir string) *Relay {
	r := &Relay{
		entries: make(map[string]*Entry),
		dataDir: dataDir,
	}
	if dataDir != "" {
		r.loadFromDisk()
	}
	return r
}

// Serve starts the relay HTTP server.
func (r *Relay) Serve(addr string, token string) error {
	mux := http.NewServeMux()

	// POST /publish — server registers/updates its URL
	mux.HandleFunc("/publish", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}

		var pub PublishRequest
		if err := json.NewDecoder(req.Body).Decode(&pub); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		if pub.ServerID == "" || pub.TunnelURL == "" || pub.Token == "" {
			http.Error(w, "server_id, tunnel_url, and token required", http.StatusBadRequest)
			return
		}

		// Per-repo auth: if this server_id already exists, the token must match
		// the one used on first publish. This prevents anyone from hijacking a server_id.
		r.mu.Lock()
		if existing, ok := r.entries[pub.ServerID]; ok && existing.Token != pub.Token {
			r.mu.Unlock()
			http.Error(w, "unauthorized: token mismatch", http.StatusUnauthorized)
			return
		}
		r.entries[pub.ServerID] = &Entry{
			ServerID:  pub.ServerID,
			TunnelURL: pub.TunnelURL,
			LANURLs:   pub.LANURLs,
			UpdatedAt: time.Now().UTC(),
			Token:     pub.Token,
		}
		r.mu.Unlock()

		r.saveToDisk()
		log.Printf("relay: published %s -> %s", pub.ServerID, pub.TunnelURL)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// GET /discover/<server_id>?token=<token> — client looks up the current URL
	// Token must match the one used to publish, so only repo members can discover.
	mux.HandleFunc("/discover/", func(w http.ResponseWriter, req *http.Request) {
		serverID := req.URL.Path[len("/discover/"):]
		if serverID == "" {
			http.Error(w, "server_id required", http.StatusBadRequest)
			return
		}

		reqToken := req.URL.Query().Get("token")

		r.mu.RLock()
		entry, exists := r.entries[serverID]
		r.mu.RUnlock()

		if !exists {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if entry.Token != reqToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	})

	// DELETE /unpublish/<server_id>?token=<token> — server removes its own entry
	mux.HandleFunc("/unpublish/", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodDelete {
			http.Error(w, "DELETE only", http.StatusMethodNotAllowed)
			return
		}
		serverID := req.URL.Path[len("/unpublish/"):]
		reqToken := req.URL.Query().Get("token")

		r.mu.Lock()
		entry, exists := r.entries[serverID]
		if !exists {
			r.mu.Unlock()
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if entry.Token != reqToken {
			r.mu.Unlock()
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		delete(r.entries, serverID)
		r.mu.Unlock()
		r.saveToDisk()
		log.Printf("relay: unpublished %s", serverID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// GET /health — simple health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		r.mu.RLock()
		count := len(r.entries)
		r.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"entries": count,
		})
	})

	log.Printf("Vibe relay listening on %s", addr)
	log.Printf("  Publish: POST %s/publish", addr)
	log.Printf("  Discover: GET %s/discover/<server_id>", addr)
	return http.ListenAndServe(addr, mux)
}

// Publish sends a URL update to a remote relay server.
func Publish(relayURL, serverID, tunnelURL, token string, lanURLs []string) error {
	pub := PublishRequest{
		ServerID:  serverID,
		TunnelURL: tunnelURL,
		LANURLs:   lanURLs,
		Token:     token,
	}
	data, err := json.Marshal(pub)
	if err != nil {
		return err
	}

	resp, err := http.Post(relayURL+"/publish", "application/json", jsonReader(data))
	if err != nil {
		return fmt.Errorf("relay publish: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("relay returned %d", resp.StatusCode)
	}
	return nil
}

// Unpublish removes a server's entry from the relay.
func Unpublish(relayURL, serverID, token string) error {
	req, err := http.NewRequest(http.MethodDelete, relayURL+"/unpublish/"+serverID+"?token="+token, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("relay unpublish: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("relay returned %d", resp.StatusCode)
	}
	return nil
}

// Discover queries a relay server for the current URL of a given server ID.
// The token must match the one used to publish — only repo members can discover.
func Discover(relayURL, serverID, token string) (*Entry, error) {
	resp, err := http.Get(relayURL + "/discover/" + serverID + "?token=" + token)
	if err != nil {
		return nil, fmt.Errorf("relay discover: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("server %s not registered at relay", serverID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("relay returned %d", resp.StatusCode)
	}

	var entry Entry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// persistence

func (r *Relay) dataFile() string {
	return filepath.Join(r.dataDir, "relay.json")
}

func (r *Relay) loadFromDisk() {
	data, err := os.ReadFile(r.dataFile())
	if err != nil {
		return
	}
	var entries map[string]*Entry
	if json.Unmarshal(data, &entries) == nil {
		r.entries = entries
	}
}

func (r *Relay) saveToDisk() {
	if r.dataDir == "" {
		return
	}
	r.mu.RLock()
	data, err := json.MarshalIndent(r.entries, "", "  ")
	r.mu.RUnlock()
	if err != nil {
		return
	}
	os.MkdirAll(r.dataDir, 0755)
	os.WriteFile(r.dataFile(), data, 0644)
}

func jsonReader(data []byte) *jsonBuf {
	return &jsonBuf{data: data}
}

type jsonBuf struct {
	data []byte
	pos  int
}

func (b *jsonBuf) Read(p []byte) (int, error) {
	if b.pos >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}
