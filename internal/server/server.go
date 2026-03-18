package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vibe-vcs/vibe/internal/core"
	"github.com/vibe-vcs/vibe/internal/roles"
)

// drop is a one-time file transfer entry.
type drop struct {
	ID        string
	Filename  string
	Data      []byte
	CreatedAt time.Time
	ExpiresAt time.Time
}

// dropStore holds pending one-time file transfers in memory.
type dropStore struct {
	mu    sync.Mutex
	drops map[string]*drop
}

func newDropStore() *dropStore {
	ds := &dropStore{drops: make(map[string]*drop)}
	go ds.reap()
	return ds
}

func (ds *dropStore) add(filename string, data []byte, ttl time.Duration) string {
	b := make([]byte, 16)
	rand.Read(b)
	id := hex.EncodeToString(b)
	ds.mu.Lock()
	ds.drops[id] = &drop{
		ID: id, Filename: filename, Data: data,
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(ttl),
	}
	ds.mu.Unlock()
	return id
}

func (ds *dropStore) take(id string) *drop {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	d, ok := ds.drops[id]
	if !ok || time.Now().After(d.ExpiresAt) {
		delete(ds.drops, id)
		return nil
	}
	delete(ds.drops, id) // one-time: delete on pickup
	return d
}

func (ds *dropStore) reap() {
	for range time.Tick(5 * time.Minute) {
		ds.mu.Lock()
		for id, d := range ds.drops {
			if time.Now().After(d.ExpiresAt) {
				delete(ds.drops, id)
			}
		}
		ds.mu.Unlock()
	}
}

// Server is the Vibe HTTP server that serves repos to linked clients.
type Server struct {
	Config  *Config
	Repo    *core.Repo
	Hub     *Hub
	Roles   *roles.Manager // nil if roles not configured
	Audit   *core.AuditLog
	Limiter *RateLimiter
	Drops   *dropStore
}

func New(cfg *Config) (*Server, error) {
	repo, err := core.FindRepo(cfg.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("repo not found at %s: %w", cfg.RepoPath, err)
	}
	srv := &Server{
		Config:  cfg,
		Repo:    repo,
		Hub:     NewHub(),
		Audit:   core.NewAuditLog(repo.VibeDir),
		Limiter: NewRateLimiter(1000, time.Minute),
		Drops:   newDropStore(),
	}
	// Check if roles are configured
	rm := roles.NewManager(repo.VibeDir)
	if _, err := rm.Load(); err == nil {
		srv.Roles = rm
		log.Printf("Role-based access control enabled")
	}
	return srv, nil
}

// ListenAndServe starts the HTTP server.
// If the port is already in use by another vibe server, it kills that process first.
func (s *Server) ListenAndServe() error {
	handler := s.buildHandler()

	addr := fmt.Sprintf("%s:%d", s.Config.Host, s.Config.Port)

	// Try to bind the port. If it fails, attempt to kill the existing vibe server.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if isAddrInUse(err) {
			log.Printf("Port %d is already in use, checking for existing vibe server...", s.Config.Port)
			if killed := killExistingVibe(s.Config.Port); killed {
				log.Printf("Stopped previous vibe server on port %d", s.Config.Port)
				// Retry bind after a short pause
				time.Sleep(500 * time.Millisecond)
				ln, err = net.Listen("tcp", addr)
				if err != nil {
					return fmt.Errorf("port %d still unavailable after stopping old server: %w", s.Config.Port, err)
				}
			} else {
				return fmt.Errorf("port %d is in use by another process (not vibe): %w", s.Config.Port, err)
			}
		} else {
			return err
		}
	}

	log.Printf("Vibe server listening on %s", addr)
	log.Printf("Serving repo: %s", s.Repo.WorkDir)
	log.Printf("WebSocket endpoint: ws://%s/ws", addr)
	log.Printf("Rate limiting: 1000 requests/min per IP")

	srv := &http.Server{Handler: handler}
	if s.Config.TLS.Enabled {
		return srv.ServeTLS(ln, s.Config.TLS.CertFile, s.Config.TLS.KeyFile)
	}
	return srv.Serve(ln)
}

// Handler returns the HTTP handler (useful for testing).
func (s *Server) Handler() http.Handler {
	return s.buildHandler()
}

func (s *Server) buildHandler() http.Handler {
	mux := http.NewServeMux()
	rl := s.Limiter.Middleware

	mux.HandleFunc("/api/info", rl(s.authMiddleware(s.handleInfo)))
	mux.HandleFunc("/api/refs", rl(s.authMiddleware(s.handleRefs)))
	mux.HandleFunc("/api/objects/", rl(s.authMiddleware(s.handleObject)))
	mux.HandleFunc("/api/tree/", rl(s.authMiddleware(s.handleTree)))
	mux.HandleFunc("/api/commit/", rl(s.authMiddleware(s.handleCommit)))
	mux.HandleFunc("/api/blob/", rl(s.authMiddleware(s.handleBlob)))
	mux.HandleFunc("/api/manifest", rl(s.authMiddleware(s.handleManifest)))
	mux.HandleFunc("/api/push", rl(s.authMiddleware(s.writeMiddleware(s.handlePush))))
	mux.HandleFunc("/api/roles", rl(s.authMiddleware(s.handleRoles)))
	mux.HandleFunc("/api/shutdown", rl(s.authMiddleware(s.writeMiddleware(s.handleShutdown))))
	mux.HandleFunc("/api/store/", rl(s.handleStore))
	mux.HandleFunc("/api/drop", rl(s.authMiddleware(s.writeMiddleware(s.handleDrop))))
	mux.HandleFunc("/api/pickup/", rl(s.handlePickup)) // no auth — token IS the credential
	mux.HandleFunc("/ws", s.authMiddleware(s.handleWebSocket)) // no rate limit on WS
	return mux
}

func (s *Server) extractToken(r *http.Request) string {
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	// Strip "Bearer " prefix if present
	if strings.HasPrefix(token, "Bearer ") {
		token = strings.TrimPrefix(token, "Bearer ")
	}
	return token
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := s.extractToken(r)

		// Role-based auth takes priority
		if s.Roles != nil {
			if token == "" {
				s.Audit.Log("auth_failed", "", "missing token", "server", r.RemoteAddr)
				http.Error(w, "unauthorized — token required", http.StatusUnauthorized)
				return
			}
			user, err := s.Roles.GetUserByToken(token)
			if err != nil {
				s.Audit.Log("auth_failed", "", "invalid token", "server", r.RemoteAddr)
				http.Error(w, "unauthorized — invalid token", http.StatusUnauthorized)
				return
			}
			if !roles.CanRead(user.Role) {
				s.Audit.Log("auth_denied", user.Name, "insufficient permissions", "server", r.RemoteAddr)
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			// Store user info in header for downstream handlers
			r.Header.Set("X-Vibe-User", user.Name)
			r.Header.Set("X-Vibe-Role", string(user.Role))
			s.Audit.Log("access", user.Name, r.Method+" "+r.URL.Path, "server", r.RemoteAddr)
			next(w, r)
			return
		}

		// Fallback: simple shared token
		if s.Config.Auth.Token != "" {
			if token != s.Config.Auth.Token {
				s.Audit.Log("auth_failed", "", "invalid shared token", "server", r.RemoteAddr)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		s.Audit.Log("access", "anonymous", r.Method+" "+r.URL.Path, "server", r.RemoteAddr)
		next(w, r)
	}
}

// writeMiddleware wraps a handler requiring write permission (admin or contributor).
func (s *Server) writeMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Roles != nil {
			roleStr := roles.Role(r.Header.Get("X-Vibe-Role"))
			if !roles.CanWrite(roleStr) {
				http.Error(w, "forbidden — write access required", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

// GET /api/info - repo metadata (includes tunnel URL + LAN addresses for re-discovery)
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	branch, headHash, _ := s.Repo.Head()
	info := map[string]interface{}{
		"branch":    branch,
		"head":      headHash.String(),
		"clients":   s.Hub.ClientCount(),
		"server_id": s.ServerID(),
		"port":      s.Config.Port,
	}
	if tunnelURL := ReadTunnelURL(s.Repo.VibeDir); tunnelURL != "" {
		info["tunnel_url"] = tunnelURL
	}
	// Include LAN addresses so clients can store them as fallbacks
	if addrs := GetLANAddresses(s.Config.Port); len(addrs) > 0 {
		info["lan_urls"] = addrs
	}
	// Include relay URL and per-repo token so clients can discover tunnel URLs.
	// The relay token is safe to share here because /api/info is auth-protected.
	relayURL := s.Config.Relay.URL
	if relayURL == "" {
		relayURL = GetDefaultRelayURL()
	}
	if relayURL != "" {
		info["relay_url"] = relayURL
	}
	if s.Config.Relay.Token != "" {
		info["relay_token"] = s.Config.Relay.Token
	}
	writeJSON(w, info)
}

// ServerID returns a stable identifier for this server's repo.
// Uses the first commit hash (immutable) or falls back to the work dir.
func (s *Server) ServerID() string {
	// Walk commit chain to find the root commit for a stable ID
	_, headHash, err := s.Repo.Head()
	if err == nil && !headHash.IsZero() {
		h := headHash
		for {
			commit, err := s.Repo.Store.ReadCommit(h)
			if err != nil || commit.ParentHash.IsZero() {
				return h.String()[:16]
			}
			h = commit.ParentHash
		}
	}
	return s.Repo.WorkDir
}

// GetLANAddresses returns http://ip:port URLs for all non-loopback IPv4 addresses.
func GetLANAddresses(port int) []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var urls []string
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			urls = append(urls, fmt.Sprintf("http://%s:%d", ipnet.IP.String(), port))
		}
	}
	return urls
}

// GET /api/refs - list all branch refs
func (s *Server) handleRefs(w http.ResponseWriter, r *http.Request) {
	refsDir := filepath.Join(s.Repo.VibeDir, "refs", "branches")
	entries, err := os.ReadDir(refsDir)
	if err != nil {
		writeJSON(w, map[string]interface{}{"refs": map[string]string{}})
		return
	}
	refs := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		meta, err := s.Repo.ReadBranchMeta(e.Name())
		if err != nil {
			continue
		}
		refs[e.Name()] = meta.Head.String()
	}
	writeJSON(w, map[string]interface{}{"refs": refs})
}

// GET /api/objects/<hash> - raw object data
func (s *Server) handleObject(w http.ResponseWriter, r *http.Request) {
	hashStr := strings.TrimPrefix(r.URL.Path, "/api/objects/")
	h, err := core.HashFromHex(hashStr)
	if err != nil {
		http.Error(w, "invalid hash", http.StatusBadRequest)
		return
	}
	if !s.Repo.Store.HasObject(h) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	objPath := filepath.Join(s.Repo.VibeDir, "objects", hashStr[:2], hashStr[2:])
	data, err := os.ReadFile(objPath)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

// GET /api/tree/<hash> - tree as JSON
func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	hashStr := strings.TrimPrefix(r.URL.Path, "/api/tree/")
	h, err := core.HashFromHex(hashStr)
	if err != nil {
		http.Error(w, "invalid hash", http.StatusBadRequest)
		return
	}
	tree, err := s.Repo.Store.ReadTree(h)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, tree)
}

// GET /api/commit/<hash> - commit as JSON
func (s *Server) handleCommit(w http.ResponseWriter, r *http.Request) {
	hashStr := strings.TrimPrefix(r.URL.Path, "/api/commit/")
	h, err := core.HashFromHex(hashStr)
	if err != nil {
		http.Error(w, "invalid hash", http.StatusBadRequest)
		return
	}
	commit, err := s.Repo.Store.ReadCommit(h)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, commit)
}

// GET /api/blob/<hash> - blob content
func (s *Server) handleBlob(w http.ResponseWriter, r *http.Request) {
	hashStr := strings.TrimPrefix(r.URL.Path, "/api/blob/")
	h, err := core.HashFromHex(hashStr)
	if err != nil {
		http.Error(w, "invalid hash", http.StatusBadRequest)
		return
	}
	data, err := s.Repo.Store.ReadBlob(h)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

// GET /api/manifest - returns current HEAD tree as a file manifest
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	_, headHash, err := s.Repo.Head()
	if err != nil || headHash.IsZero() {
		writeJSON(w, map[string]interface{}{"files": map[string]interface{}{}})
		return
	}
	commit, err := s.Repo.Store.ReadCommit(headHash)
	if err != nil {
		http.Error(w, "read commit", http.StatusInternalServerError)
		return
	}
	tree, err := s.Repo.Store.ReadTree(commit.TreeHash)
	if err != nil {
		http.Error(w, "read tree", http.StatusInternalServerError)
		return
	}

	files := make(map[string]interface{})
	for _, entry := range tree.Entries {
		f := map[string]interface{}{
			"hash": entry.Hash.String(),
			"mode": entry.Mode,
		}
		if size, err := s.Repo.Store.BlobSize(entry.Hash); err == nil {
			f["size"] = size
		}
		files[entry.Name] = f
	}

	branch, _, _ := s.Repo.Head()
	writeJSON(w, map[string]interface{}{
		"branch": branch,
		"head":   headHash.String(),
		"files":  files,
	})
}

// POST /api/push - receive a new commit from a contributor (push changes to server)
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	var pushData struct {
		Objects map[string][]byte `json:"objects"` // hash -> raw object bytes
		Branch  string            `json:"branch"`
		Head    string            `json:"head"` // new head commit hash
	}
	if err := json.Unmarshal(body, &pushData); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Write objects to store
	for hashStr, data := range pushData.Objects {
		objPath := filepath.Join(s.Repo.VibeDir, "objects", hashStr[:2], hashStr[2:])
		if _, err := os.Stat(objPath); err == nil {
			continue // already have it
		}
		os.MkdirAll(filepath.Dir(objPath), 0755)
		if err := os.WriteFile(objPath, data, 0444); err != nil {
			http.Error(w, fmt.Sprintf("write object: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Update branch ref
	if pushData.Branch != "" && pushData.Head != "" {
		h, err := core.HashFromHex(pushData.Head)
		if err != nil {
			http.Error(w, "invalid head hash", http.StatusBadRequest)
			return
		}
		if err := s.Repo.UpdateRef(pushData.Branch, h); err != nil {
			http.Error(w, fmt.Sprintf("update ref: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Audit the push
	user := r.Header.Get("X-Vibe-User")
	if user == "" {
		user = "anonymous"
	}
	s.Audit.Log("push", user, fmt.Sprintf("branch=%s head=%s objects=%d", pushData.Branch, pushData.Head, len(pushData.Objects)), "server", r.RemoteAddr)

	// Broadcast to all connected clients
	s.Hub.Broadcast(&Event{
		Type:    "commit",
		Branch:  pushData.Branch,
		Hash:    pushData.Head,
		Message: "New changes pushed",
	})

	writeJSON(w, map[string]string{"status": "ok"})
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// GET /ws - WebSocket connection for live push notifications
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}

	client := s.Hub.Register(conn)
	log.Printf("Client connected (total: %d)", s.Hub.ClientCount())

	// Send initial state (includes tunnel URL for re-discovery)
	branch, headHash, _ := s.Repo.Head()
	tunnelURL := ReadTunnelURL(s.Repo.VibeDir)
	welcome := &Event{
		Type:    "connected",
		Branch:  branch,
		Hash:    headHash.String(),
		Message: "Connected to Vibe server",
	}
	if tunnelURL != "" {
		welcome.Data = map[string]string{"tunnel_url": tunnelURL}
	}
	data, _ := json.Marshal(welcome)
	client.send <- data

	// Block on read pump (detects disconnect)
	client.ReadPump()
	log.Printf("Client disconnected (total: %d)", s.Hub.ClientCount())
}

// GET /api/roles - list users and roles (admin only sees tokens, others see names+roles)
func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	if s.Roles == nil {
		writeJSON(w, map[string]string{"error": "roles not configured"})
		return
	}
	rf, err := s.Roles.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	roleStr := roles.Role(r.Header.Get("X-Vibe-Role"))
	type userInfo struct {
		Name  string `json:"name"`
		Role  string `json:"role"`
		Token string `json:"token,omitempty"`
	}
	var users []userInfo
	for _, u := range rf.Users {
		ui := userInfo{Name: u.Name, Role: string(u.Role)}
		if roles.CanManage(roleStr) {
			ui.Token = u.Token
		}
		users = append(users, ui)
	}
	writeJSON(w, map[string]interface{}{"owner": rf.Owner, "users": users})
}

// BroadcastTunnelUpdate notifies all connected WebSocket clients of a new tunnel URL.
// Clients use this to update their stored server URL automatically.
func (s *Server) BroadcastTunnelUpdate(tunnelURL string) {
	s.Hub.Broadcast(&Event{
		Type:    "tunnel_update",
		Message: "Server tunnel URL changed",
		Data:    map[string]string{"tunnel_url": tunnelURL},
	})
	log.Printf("Broadcast tunnel_update to %d client(s): %s", s.Hub.ClientCount(), tunnelURL)
}

// POST /api/shutdown — broadcasts a repo_shutdown warning to all clients, then signals the server to stop.
// Requires write permission. Grace period (seconds) can be passed as ?grace=30.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	grace := 30
	if v := r.URL.Query().Get("grace"); v != "" {
		fmt.Sscanf(v, "%d", &grace)
	}
	s.Hub.Broadcast(&Event{
		Type:    "repo_shutdown",
		Message: fmt.Sprintf("This repo is being deleted. You have %d seconds to save a copy.", grace),
		Data:    map[string]int{"grace_seconds": grace},
	})
	log.Printf("Broadcast repo_shutdown to %d client(s), grace=%ds", s.Hub.ClientCount(), grace)
	writeJSON(w, map[string]interface{}{"status": "broadcast_sent", "grace_seconds": grace, "clients": s.Hub.ClientCount()})
}

// /api/store/ — persistent file storage, not tracked by the VCS.
// Files live in .vibe/store/ and are served/managed here.
//
//	GET    /api/store/          — list files (auth required)
//	GET    /api/store/<name>    — download a file (auth required)
//	POST   /api/store/<name>    — upload a file (write access required)
//	DELETE /api/store/<name>    — delete a file (write access required)
func (s *Server) handleStore(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/store/")
	storeDir := filepath.Join(s.Repo.VibeDir, "store")

	// Auth check
	token := s.extractToken(r)
	if s.Roles != nil {
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		user, err := s.Roles.GetUserByToken(token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost || r.Method == http.MethodDelete {
			if !roles.CanWrite(user.Role) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
	}

	switch r.Method {
	case http.MethodGet:
		if name == "" {
			// List files
			entries, err := os.ReadDir(storeDir)
			if err != nil {
				writeJSON(w, map[string]interface{}{"files": []string{}})
				return
			}
			type fileEntry struct {
				Name string `json:"name"`
				Size int64  `json:"size"`
			}
			var files []fileEntry
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				info, _ := e.Info()
				files = append(files, fileEntry{Name: e.Name(), Size: info.Size()})
			}
			if files == nil {
				files = []fileEntry{}
			}
			writeJSON(w, map[string]interface{}{"files": files})
		} else {
			// Download
			filePath := filepath.Join(storeDir, filepath.Base(name))
			data, err := os.ReadFile(filePath)
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filepath.Base(name)))
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(data)
		}

	case http.MethodPost:
		if name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(storeDir, 0755); err != nil {
			http.Error(w, "store error", http.StatusInternalServerError)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		filePath := filepath.Join(storeDir, filepath.Base(name))
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}
		log.Printf("store: uploaded %s (%d bytes)", name, len(data))
		writeJSON(w, map[string]string{"status": "ok", "name": filepath.Base(name)})

	case http.MethodDelete:
		if name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		filePath := filepath.Join(storeDir, filepath.Base(name))
		if err := os.Remove(filePath); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		log.Printf("store: deleted %s", name)
		writeJSON(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// POST /api/drop — upload a file for one-time pickup.
// Multipart form: field "file". Optional query: ?ttl=24h (default 24h).
// Returns a pickup URL the sender can share.
func (s *Server) handleDrop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	ttl := 24 * time.Hour
	if v := r.URL.Query().Get("ttl"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			ttl = d
		}
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	id := s.Drops.add(header.Filename, data, ttl)
	// Build pickup URL using the tunnel URL if available, else server address
	base := ReadTunnelURL(s.Repo.VibeDir)
	if base == "" {
		base = fmt.Sprintf("http://localhost:%d", s.Config.Port)
	}
	pickupURL := fmt.Sprintf("%s/api/pickup/%s", base, id)
	log.Printf("drop: %s (%d bytes, ttl %s) -> %s", header.Filename, len(data), ttl, id)
	writeJSON(w, map[string]string{
		"id":         id,
		"pickup_url": pickupURL,
		"expires_in": ttl.String(),
		"command":    fmt.Sprintf("vibe pickup %s", pickupURL),
	})
}

// GET /api/pickup/<id> — download a dropped file exactly once, then it's gone.
func (s *Server) handlePickup(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/pickup/")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	d := s.Drops.take(id)
	if d == nil {
		http.Error(w, "not found or already picked up", http.StatusNotFound)
		return
	}
	log.Printf("pickup: served %s (%d bytes) — deleted", d.Filename, len(d.Data))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filepath.Base(d.Filename)))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(d.Data)))
	w.Write(d.Data)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
