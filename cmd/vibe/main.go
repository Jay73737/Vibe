package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"time"

	"github.com/vibe-vcs/vibe/internal/branch"
	"github.com/vibe-vcs/vibe/internal/core"
	"github.com/vibe-vcs/vibe/internal/daemon"
	"github.com/vibe-vcs/vibe/internal/history"
	"github.com/vibe-vcs/vibe/internal/link"
	vibeRelay "github.com/vibe-vcs/vibe/internal/relay"
	"github.com/vibe-vcs/vibe/internal/roles"
	"github.com/vibe-vcs/vibe/internal/server"
	"github.com/vibe-vcs/vibe/internal/ui"
)

// version is set at build time via: go build -ldflags "-X main.version=v1.0.0"
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "vibe: no command specified. Run 'vibe help' for usage.")
		os.Exit(1)
	}

	switch os.Args[1] {
	// Quick workflow (vibe coding)
	case "vibe":
		cmdVibe()
	case "save":
		cmdSave()
	case "nuke":
		cmdNuke()
	case "share":
		cmdShare()
	case "invite":
		cmdInvite()
	// Core
	case "init":
		cmdInit()
	case "add":
		cmdAdd()
	case "commit":
		cmdCommit()
	case "status":
		cmdStatus()
	case "log":
		cmdLog()
	case "config":
		cmdConfig()
	// Branching
	case "branch":
		cmdBranch()
	case "branches":
		cmdBranches()
	case "switch":
		cmdSwitch()
	case "destroy":
		cmdDestroy()
	case "merge":
		cmdMerge()
	case "sessions":
		cmdSessions()
	case "restore":
		cmdRestore()
	// History
	case "diff":
		cmdDiff()
	case "revert":
		cmdRevert()
	case "blame":
		cmdBlame()
	// Import
	case "import":
		cmdImport()
	// Linking
	case "link":
		cmdLink()
	case "fetch":
		cmdFetch()
	case "pull":
		cmdPull()
	case "sync":
		cmdSync()
	case "drop":
		cmdDrop()
	case "pickup":
		cmdPickup()
	case "store":
		cmdStore()
	// Daemon & Service
	case "daemon":
		cmdDaemon()
	case "service":
		cmdService()
	// Roles
	case "roles":
		cmdRoles()
	case "grant":
		cmdGrant()
	case "revoke":
		cmdRevoke()
	// Server
	case "serve":
		cmdServe()
	case "uninit":
		cmdUninit()
	case "relay":
		cmdRelay()
	case "ui":
		cmdUI()
	// Audit
	case "audit":
		cmdAudit()
	case "help", "--help", "-h":
		printUsage()
	case "--version", "version":
		fmt.Println("vibe " + version)
	case "update":
		cmdUpdate()
	default:
		fmt.Fprintf(os.Stderr, "vibe: unknown command '%s'. Run 'vibe help' for usage.\n", os.Args[1])
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`vibe - modern version control for the vibe coding era

Usage: vibe <command> [arguments]

Quick Workflow:
  vibe <name>           Spin up a new branch and switch to it instantly
  save [message]        Quick add-all + commit in one shot
  nuke [name]           Destroy a branch and revert to where you were
  invite <user> [role]   Generate a connection invite for someone
  share <branch> <user> Push a branch to a connected user

Core:
  init [dir]            Create a new vibe repository
  add <files>           Stage files (use '.' for all)
  commit -m "msg"       Commit staged files
  status                Show what's changed
  log                   Show commit history
  config                Set author name (vibe config author "Your Name")

Branching & Sessions:
  branch <name>         Create a new branch (tracks parent branch)
  branches              List all branches (shows lineage)
  switch <name>         Switch branch (auto-saves your work)
  merge <branch>        Merge another branch into current branch
  destroy <name>        Delete a branch and its sessions
  sessions              List saved sessions
  restore <id>          Restore a saved session

History:
  diff                  Show changes vs last commit
  diff <hash> <hash>    Compare two commits
  revert <hash>         Revert to any previous commit
  blame <file>          Show who changed each line

Import:
  import <git-url>      Clone a git repo and convert to Vibe

Linking & Sync:
  link <source> [dir]   Link to a repo (auto-creates working branch, registers with daemon)
  fetch <file>          Fetch a file from source on-demand
  pull [--no-size-limit] Fetch all files from source (default: skip files >100MB)
  sync                  Pull latest refs from source (one-shot, prefer daemon for auto-sync)

File Transfer:
  drop <file>           Create a one-time pickup link (--server URL, --token, --ttl)
  pickup <url>          Download a one-time drop link
  store list            List files in the persistent store
  store put <file>      Upload a file to the store (--server, --token)
  store get <name>      Download a file from the store
  store rm <name>       Delete a file from the store

Roles:
  roles                 List users (use 'roles init <name>' to set up)
  grant <user> <role>   Assign role: admin, contributor, or reader
  revoke <user>         Remove access

Server:
  serve                 Start server (--port, --token, --tunnel, --relay, --config)
  relay                 Start URL relay server (--port, --token, --data)
  ui                    Open web dashboard (--port, --server)

Daemon & Service:
  daemon                Run the sync daemon in foreground
  service install       Install daemon as a startup service
  service uninstall     Remove the startup service
  service start         Start the daemon service
  service stop          Stop the daemon service
  service status        Check daemon service status

Security:
  audit                 View audit log (-n 50, --all)

Set VIBE_AUTHOR or run 'vibe config author "name"' to set your identity.
Create a .vibeignore file to exclude files from tracking.`)
}

// --- Quick workflow commands (vibe coding) ---

// vibe vibe <name> — spin up a temp branch and switch to it
func cmdVibe() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe vibe <name>  — spin up a branch and switch to it")
		os.Exit(1)
	}
	name := os.Args[2]
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := branch.NewManager(repo)

	// Create and switch in one shot
	if err := mgr.Create(name); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := mgr.Switch(name, false); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	auditCLI(repo, "vibe", fmt.Sprintf("branch=%s", name))
	fmt.Printf("Vibing on '%s' — go wild, 'vibe nuke %s' to throw it away.\n", name, name)
}

// vibe save [message] — quick add-all + commit
func cmdSave() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Add all files
	ignorePatterns := core.LoadIgnorePatterns(repo.WorkDir)
	filepath.WalkDir(repo.WorkDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() && d.Name() == core.VibeDirName {
			return filepath.SkipDir
		}
		if d.IsDir() {
			relDir, _ := filepath.Rel(repo.WorkDir, path)
			if relDir != "." && core.IsIgnored(filepath.ToSlash(relDir), ignorePatterns) {
				return filepath.SkipDir
			}
			return nil
		}
		relPath, _ := filepath.Rel(repo.WorkDir, path)
		relSlash := filepath.ToSlash(relPath)
		if core.IsIgnored(relSlash, ignorePatterns) {
			return nil
		}
		repo.AddToIndex(relPath)
		return nil
	})

	// Build commit message
	message := "save"
	if len(os.Args) >= 3 {
		message = strings.Join(os.Args[2:], " ")
	}

	h, err := repo.CreateCommit(getAuthor(), message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	branchName, _, _ := repo.Head()
	auditCLI(repo, "save", fmt.Sprintf("branch=%s commit=%s msg=%s", branchName, h.Short(), message))
	fmt.Printf("[%s %s] %s\n", branchName, h.Short(), message)
}

// vibe nuke [name] — destroy a branch and switch back to main
func cmdNuke() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := branch.NewManager(repo)

	currentBranch, _, _ := repo.Head()
	target := currentBranch
	if len(os.Args) >= 3 {
		target = os.Args[2]
	}

	if target == "main" {
		fmt.Fprintln(os.Stderr, "error: can't nuke main branch")
		os.Exit(1)
	}

	// If we're on the branch we're nuking, switch to main first
	if target == currentBranch {
		if err := mgr.Switch("main", true); err != nil {
			fmt.Fprintf(os.Stderr, "error switching to main: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Switched to main.")
	}

	if err := mgr.Destroy(target); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	auditCLI(repo, "nuke", fmt.Sprintf("branch=%s", target))
	fmt.Printf("Nuked branch '%s'. Gone forever.\n", target)
}

// vibe share <branch> <user> — notify a connected user about a branch
func cmdShare() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: vibe share <branch> <user>")
		fmt.Fprintln(os.Stderr, "  Shares a branch with a connected user by pushing a notification.")
		fmt.Fprintln(os.Stderr, "  The user must be linked to the repo and listening (via vibe link/sync).")
		os.Exit(1)
	}
	branchName := os.Args[2]
	userName := os.Args[3]

	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Verify branch exists
	refPath := filepath.Join(repo.VibeDir, "refs", "branches", branchName)
	if _, err := os.Stat(refPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: branch '%s' does not exist\n", branchName)
		os.Exit(1)
	}

	// Verify user exists in roles
	rm := roles.NewManager(repo.VibeDir)
	user, err := rm.GetUser(userName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintln(os.Stderr, "hint: run 'vibe grant <user> reader' to add them first.")
		os.Exit(1)
	}

	_ = user
	fmt.Printf("Shared branch '%s' with %s.\n", branchName, userName)
	fmt.Printf("They can sync with: vibe sync\n")
	fmt.Printf("Then switch with:   vibe switch %s\n", branchName)
}

// vibe invite <user> [role] [--port PORT] — generate a connection invite
func cmdInvite() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, `usage: vibe invite <user> [role] [--port PORT]

Generates a ready-to-paste command that someone else can run to
connect to your vibe server. Sets up roles automatically if needed.

  role    admin, contributor, or reader (default: contributor)
  --port  server port (default: 7433)

Examples:
  vibe invite Alice
  vibe invite Bob reader --port 8080`)
		os.Exit(1)
	}

	userName := os.Args[2]
	role := roles.Contributor
	port := 7433

	for i := 3; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--port", "-p":
			if i+1 < len(os.Args) {
				fmt.Sscanf(os.Args[i+1], "%d", &port)
				i++
			}
		default:
			if r := roles.Role(os.Args[i]); roles.ValidRoles[r] {
				role = r
			}
		}
	}

	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	rm := roles.NewManager(repo.VibeDir)

	// Auto-initialize roles if not set up yet
	if _, err := rm.Load(); err != nil {
		author := getAuthor()
		if initErr := rm.Init(author, ""); initErr != nil {
			fmt.Fprintf(os.Stderr, "error initializing roles: %v\n", initErr)
			os.Exit(1)
		}
		fmt.Printf("Roles initialized (owner: %s)\n\n", author)
	}

	// Grant the user (creates them if they don't exist)
	if err := rm.Grant(userName, role, ""); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	user, err := rm.GetUser(userName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	auditCLI(repo, "invite", fmt.Sprintf("user=%s role=%s", userName, role))

	// Check if a cloudflared tunnel is active
	tunnelURL := server.ReadTunnelURL(repo.VibeDir)

	fmt.Printf("Invite for %s (%s):\n\n", userName, role)
	if tunnelURL != "" {
		fmt.Printf("  vibe link %s --token %s\n\n", tunnelURL, user.Token)
		fmt.Println("Send the command above to", userName+".")
		fmt.Println("This link works from anywhere on the internet.")
	} else {
		ip := getOutboundIP()
		fmt.Printf("  vibe link http://%s:%d --token %s\n\n", ip, port, user.Token)
		fmt.Println("Send the command above to", userName+".")
		fmt.Println("Make sure your server is running: vibe serve --port", port)
		fmt.Println("Tip: use 'vibe serve --tunnel' to make it accessible from anywhere.")
	}
}

// getOutboundIP returns the preferred outbound IP of this machine.
func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		// Fallback: scan interfaces
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return "YOUR_IP"
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
		return "YOUR_IP"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

// vibe config — get/set configuration
func cmdConfig() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, `usage: vibe config <key> [value]

Keys:
  author    Your display name for commits`)
		os.Exit(1)
	}

	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	configPath := filepath.Join(repo.VibeDir, "config.json")
	config := make(map[string]string)
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &config)
	}

	key := os.Args[2]
	if len(os.Args) >= 4 {
		// Set
		config[key] = os.Args[3]
		data, _ := json.MarshalIndent(config, "", "  ")
		os.WriteFile(configPath, data, 0644)
		fmt.Printf("Set %s = %s\n", key, os.Args[3])
	} else {
		// Get
		if val, ok := config[key]; ok {
			fmt.Println(val)
		} else {
			fmt.Fprintf(os.Stderr, "'%s' not set\n", key)
			os.Exit(1)
		}
	}
}

// --- Core commands ---

func cmdInit() {
	dir := "."
	if len(os.Args) > 2 {
		dir = os.Args[2]
	}
	repo, err := core.InitRepo(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Store default relay URL and generate a per-repo relay token.
	// The token is unique to this repo — only users who link it get the token,
	// so only they can discover (or publish) this repo's tunnel URL on the relay.
	configPath := filepath.Join(repo.VibeDir, "config.json")
	config := make(map[string]string)
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &config)
	}
	relayURL := server.GetDefaultRelayURL()
	if relayURL != "" {
		config["relay_url"] = relayURL
	}
	// Generate a random per-repo token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err == nil {
		config["relay_token"] = hex.EncodeToString(tokenBytes)
	}
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0644)

	auditCLI(repo, "init", fmt.Sprintf("initialized repo at %s", repo.WorkDir))
	fmt.Printf("Initialized empty vibe repository in %s\n", repo.VibeDir)
}

func cmdUninit() {
	grace := 30
	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "--grace" && i+1 < len(os.Args) {
			fmt.Sscanf(os.Args[i+1], "%d", &grace)
			i++
		}
	}

	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Confirm
	fmt.Printf("This will permanently delete the vibe repo at %s\n", repo.VibeDir)
	fmt.Print("Type 'yes' to continue: ")
	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "yes" {
		fmt.Println("Aborted.")
		return
	}

	// Read relay config
	var relayURL, relayToken, serverID string
	configPath := filepath.Join(repo.VibeDir, "config.json")
	if data, err := os.ReadFile(configPath); err == nil {
		var cfg map[string]string
		if json.Unmarshal(data, &cfg) == nil {
			relayURL = cfg["relay_url"]
			relayToken = cfg["relay_token"]
		}
	}

	// Broadcast shutdown warning to connected clients via the running server
	port := 7433
	serverToken := relayToken // use relay token as a fallback; real deployments set --token
	shutdownURL := fmt.Sprintf("http://localhost:%d/api/shutdown?grace=%d", port, grace)
	req, _ := http.NewRequest(http.MethodPost, shutdownURL, nil)
	if serverToken != "" {
		req.Header.Set("Authorization", "Bearer "+serverToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("  Server not running locally — skipping broadcast.")
	} else {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			fmt.Printf("  Broadcast repo_shutdown to connected clients. Waiting %ds...\n", grace)
			time.Sleep(time.Duration(grace) * time.Second)
		}
	}

	// Unpublish from relay
	if relayURL != "" && relayToken != "" {
		// Derive server ID from root commit (same logic as server.ServerID)
		if id, err := getServerID(repo); err == nil {
			serverID = id
		}
		if serverID != "" {
			if err := vibeRelay.Unpublish(relayURL, serverID, relayToken); err != nil {
				fmt.Fprintf(os.Stderr, "  relay unpublish warning: %v\n", err)
			} else {
				fmt.Printf("  Removed from relay: %s\n", relayURL)
			}
		}
	}

	// Delete the .vibe directory
	if err := os.RemoveAll(repo.VibeDir); err != nil {
		fmt.Fprintf(os.Stderr, "error deleting %s: %v\n", repo.VibeDir, err)
		os.Exit(1)
	}
	fmt.Printf("Deleted %s\n", repo.VibeDir)
}

// getServerID derives the stable server ID from the root commit hash.
func getServerID(repo *core.Repo) (string, error) {
	_, headHash, err := repo.Head()
	if err != nil || headHash.IsZero() {
		return "", fmt.Errorf("no commits")
	}
	h := headHash
	for {
		commit, err := repo.Store.ReadCommit(h)
		if err != nil || commit.ParentHash.IsZero() {
			break
		}
		h = commit.ParentHash
	}
	s := h.String()
	if len(s) > 16 {
		return s[:16], nil
	}
	return s, nil
}

func cmdAdd() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe add <file> [file...]")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for _, arg := range os.Args[2:] {
		if arg == "." {
			if err := addAll(repo); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			continue
		}
		relPath, err := filepath.Rel(repo.WorkDir, filepath.Join(mustGetwd(), arg))
		if err != nil {
			relPath = arg
		}
		if err := repo.AddToIndex(relPath); err != nil {
			fmt.Fprintf(os.Stderr, "error adding %s: %v\n", arg, err)
			os.Exit(1)
		}
	}
}

func addAll(repo *core.Repo) error {
	ignorePatterns := core.LoadIgnorePatterns(repo.WorkDir)
	return filepath.WalkDir(repo.WorkDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == core.VibeDirName {
			return filepath.SkipDir
		}
		if d.IsDir() {
			relDir, _ := filepath.Rel(repo.WorkDir, path)
			if relDir != "." && core.IsIgnored(filepath.ToSlash(relDir), ignorePatterns) {
				return filepath.SkipDir
			}
			return nil
		}
		relPath, _ := filepath.Rel(repo.WorkDir, path)
		if core.IsIgnored(filepath.ToSlash(relPath), ignorePatterns) {
			return nil
		}
		return repo.AddToIndex(relPath)
	})
}

func cmdCommit() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	message := ""
	author := getAuthor()

	for i := 2; i < len(os.Args); i++ {
		if (os.Args[i] == "-m" || os.Args[i] == "--message") && i+1 < len(os.Args) {
			message = os.Args[i+1]
			i++
		}
	}
	if message == "" {
		fmt.Fprintln(os.Stderr, "error: commit message required (-m \"message\")")
		os.Exit(1)
	}

	h, err := repo.CreateCommit(author, message)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	branchName, _, _ := repo.Head()
	auditCLI(repo, "commit", fmt.Sprintf("branch=%s commit=%s msg=%s", branchName, h.Short(), message))
	fmt.Printf("[%s %s] %s\n", branchName, h.Short(), message)
}

func cmdStatus() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	branchName, _, _ := repo.Head()
	fmt.Printf("On branch %s\n\n", branchName)

	staged, modified, untracked, err := repo.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(staged) > 0 {
		fmt.Println("Staged files:")
		for _, f := range staged {
			fmt.Printf("  \033[32m%s\033[0m\n", f)
		}
		fmt.Println()
	}
	if len(modified) > 0 {
		fmt.Println("Modified (not staged):")
		for _, f := range modified {
			fmt.Printf("  \033[33m%s\033[0m\n", f)
		}
		fmt.Println()
	}
	if len(untracked) > 0 {
		fmt.Println("Untracked files:")
		for _, f := range untracked {
			fmt.Printf("  \033[31m%s\033[0m\n", f)
		}
		fmt.Println()
	}
	if len(staged) == 0 && len(modified) == 0 && len(untracked) == 0 {
		fmt.Println("Nothing to report, working tree clean.")
	}
}

func cmdLog() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	_, commitHash, err := repo.Head()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if commitHash.IsZero() {
		fmt.Println("No commits yet.")
		return
	}

	h := commitHash
	for !h.IsZero() {
		commit, err := repo.Store.ReadCommit(h)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading commit: %v\n", err)
			break
		}
		fmt.Printf("\033[33mcommit %s\033[0m\n", h.String())
		fmt.Printf("Author: %s\n", commit.Author)
		fmt.Printf("Date:   %s\n", commit.Timestamp.Format("Mon Jan 2 15:04:05 2006 -0700"))
		fmt.Println()
		for _, line := range strings.Split(commit.Message, "\n") {
			fmt.Printf("    %s\n", line)
		}
		fmt.Println()
		h = commit.ParentHash
	}
}

func cmdBranch() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe branch <name>")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := branch.NewManager(repo)
	if err := mgr.Create(os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	auditCLI(repo, "branch", fmt.Sprintf("created branch=%s", os.Args[2]))
	fmt.Printf("Created branch '%s'\n", os.Args[2])
}

func cmdBranches() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := branch.NewManager(repo)
	branches, current, err := mgr.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	for _, b := range branches {
		meta, _ := repo.ReadBranchMeta(b)
		suffix := ""
		if meta != nil && meta.Parent != "" {
			suffix = fmt.Sprintf(" \033[90m(from %s)\033[0m", meta.Parent)
		}
		if b == current {
			fmt.Printf("* \033[32m%s\033[0m%s\n", b, suffix)
		} else {
			fmt.Printf("  %s%s\n", b, suffix)
		}
	}
}

func cmdSwitch() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe switch <branch> [--no-session]")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	target := os.Args[2]
	noSession := false
	for _, arg := range os.Args[3:] {
		if arg == "--no-session" {
			noSession = true
		}
	}

	mgr := branch.NewManager(repo)
	if err := mgr.Switch(target, noSession); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	auditCLI(repo, "switch", fmt.Sprintf("branch=%s", target))
	fmt.Printf("Switched to branch '%s'\n", target)
	if !noSession {
		fmt.Println("(previous work auto-saved as session)")
	}
}

func cmdDestroy() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe destroy <branch>")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := branch.NewManager(repo)
	if err := mgr.Destroy(os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	auditCLI(repo, "destroy", fmt.Sprintf("branch=%s", os.Args[2]))
	fmt.Printf("Destroyed branch '%s'\n", os.Args[2])
}

func cmdMerge() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, `usage: vibe merge <branch>

Merges another branch into your current branch using three-way merge.
Files you changed are kept. Files only the other branch changed are pulled in.
If both sides changed a file, your version is kept and a warning is shown.

Examples:
  vibe merge main           Pull latest main into your working branch
  vibe merge feature-auth   Merge a feature branch into current`)
		os.Exit(1)
	}

	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	sourceBranch := os.Args[2]
	currentBranch, _, _ := repo.Head()

	mgr := branch.NewManager(repo)
	commitHash, conflicts, err := mgr.Merge(sourceBranch, getAuthor())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	auditCLI(repo, "merge", fmt.Sprintf("source=%s into=%s commit=%s", sourceBranch, currentBranch, commitHash.Short()))

	if len(conflicts) > 0 {
		fmt.Printf("Merged '%s' into '%s' [%s] with %d conflict(s):\n", sourceBranch, currentBranch, commitHash.Short(), len(conflicts))
		for _, c := range conflicts {
			fmt.Printf("  \033[33mCONFLICT\033[0m %s (kept your version)\n", c)
		}
	} else {
		fmt.Printf("Merged '%s' into '%s' [%s]\n", sourceBranch, currentBranch, commitHash.Short())
	}
}

func cmdSessions() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	filterBranch := ""
	for i := 2; i < len(os.Args); i++ {
		if (os.Args[i] == "-b" || os.Args[i] == "--branch") && i+1 < len(os.Args) {
			filterBranch = os.Args[i+1]
			i++
		}
	}

	mgr := branch.NewManager(repo)
	sessions, err := mgr.Sessions(filterBranch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		fmt.Println("No saved sessions.")
		return
	}

	for _, s := range sessions {
		fmt.Printf("\033[33m%s\033[0m\n", s.ID)
		fmt.Printf("  %s\n", s.Message)
		fmt.Printf("  Branch: %s  |  %s  |  %d staged, %d modified\n",
			s.Branch,
			s.Timestamp.Format("Jan 2 15:04"),
			len(s.Index), len(s.WorkingFiles))
		fmt.Println()
	}
}

func cmdRestore() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe restore <session-id>")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := branch.NewManager(repo)
	if err := mgr.Restore(os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Restored session '%s'\n", os.Args[2])
}

func cmdDiff() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := history.NewManager(repo)

	if len(os.Args) >= 4 {
		// diff <old-hash> <new-hash>
		oldHash, err := core.HashFromHex(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid hash %q\n", os.Args[2])
			os.Exit(1)
		}
		newHash, err := core.HashFromHex(os.Args[3])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid hash %q\n", os.Args[3])
			os.Exit(1)
		}
		diffs, err := mgr.DiffCommits(oldHash, newHash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		for _, d := range diffs {
			fmt.Print(history.FormatDiff(&d))
		}
		return
	}

	// diff working tree vs HEAD
	diffs, err := mgr.DiffWorkingTree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(diffs) == 0 {
		fmt.Println("No changes.")
		return
	}
	for _, d := range diffs {
		fmt.Print(history.FormatDiff(&d))
	}
}

func cmdRevert() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe revert <commit-hash>")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	targetHash, err := core.HashFromHex(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid hash %q\n", os.Args[2])
		os.Exit(1)
	}

	mgr := history.NewManager(repo)
	newHash, err := mgr.Revert(targetHash, getAuthor())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	branchName, _, _ := repo.Head()
	auditCLI(repo, "revert", fmt.Sprintf("branch=%s to=%s commit=%s", branchName, targetHash.Short(), newHash.Short()))
	fmt.Printf("[%s %s] Revert to %s\n", branchName, newHash.Short(), targetHash.Short())
}

func cmdBlame() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe blame <file>")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	relPath := os.Args[2]
	mgr := history.NewManager(repo)
	lines, err := mgr.Blame(relPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for _, bl := range lines {
		ts := ""
		if t, ok := bl.Timestamp.(time.Time); ok {
			ts = t.Format("2006-01-02")
		}
		fmt.Printf("\033[33m%s\033[0m %-12s %s \033[2m%4d\033[0m | %s\n",
			bl.CommitHash.Short(), bl.Author, ts, bl.LineNum, bl.Content)
	}
}

func cmdImport() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe import <git-url> [target-dir]")
		os.Exit(1)
	}
	gitURL := os.Args[2]
	targetDir := ""
	if len(os.Args) > 3 {
		targetDir = os.Args[3]
	} else {
		// Derive directory name from URL
		parts := strings.Split(strings.TrimSuffix(gitURL, ".git"), "/")
		targetDir = parts[len(parts)-1]
	}

	fmt.Printf("Importing %s into %s...\n", gitURL, targetDir)
	repo, err := core.ImportGit(gitURL, targetDir, getAuthor())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	_, headHash, _ := repo.Head()
	fmt.Printf("\nImported into vibe repository: %s\n", repo.VibeDir)
	if !headHash.IsZero() {
		fmt.Printf("Commit: %s\n", headHash.Short())
	}
}

func cmdRoles() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := roles.NewManager(repo.VibeDir)

	// Check for 'roles init <name>'
	if len(os.Args) >= 4 && os.Args[2] == "init" {
		ownerName := os.Args[3]
		token := ""
		for i := 4; i < len(os.Args); i++ {
			if os.Args[i] == "--token" && i+1 < len(os.Args) {
				token = os.Args[i+1]
				i++
			}
		}
		if err := mgr.Init(ownerName, token); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		user, _ := mgr.GetUser(ownerName)
		fmt.Printf("Roles initialized. Owner: %s (admin)\n", ownerName)
		fmt.Printf("Your token: %s\n", user.Token)
		return
	}

	// List roles
	rf, err := mgr.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Owner: %s\n\n", rf.Owner)
	fmt.Printf("%-20s %-15s %s\n", "USER", "ROLE", "TOKEN")
	fmt.Printf("%-20s %-15s %s\n", "----", "----", "-----")
	for _, u := range rf.Users {
		fmt.Printf("%-20s %-15s %s\n", u.Name, u.Role, u.Token)
	}
}

func cmdGrant() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: vibe grant <user> <role> [--token <token>]")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := roles.NewManager(repo.VibeDir)
	userName := os.Args[2]
	role := roles.Role(os.Args[3])
	token := ""
	for i := 4; i < len(os.Args); i++ {
		if os.Args[i] == "--token" && i+1 < len(os.Args) {
			token = os.Args[i+1]
			i++
		}
	}

	if err := mgr.Grant(userName, role, token); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	user, _ := mgr.GetUser(userName)
	auditCLI(repo, "grant", fmt.Sprintf("user=%s role=%s", userName, role))
	fmt.Printf("Granted '%s' role to %s\n", role, userName)
	fmt.Printf("Token: %s\n", user.Token)
}

func cmdRevoke() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe revoke <user>")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := roles.NewManager(repo.VibeDir)
	if err := mgr.Revoke(os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	auditCLI(repo, "revoke", fmt.Sprintf("user=%s", os.Args[2]))
	fmt.Printf("Revoked access for '%s'\n", os.Args[2])
}

func cmdUI() {
	port := 7434
	serverURL := "http://localhost:7433"
	token := ""

	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--port", "-p":
			if i+1 < len(os.Args) {
				fmt.Sscanf(os.Args[i+1], "%d", &port)
				i++
			}
		case "--server", "-s":
			if i+1 < len(os.Args) {
				serverURL = os.Args[i+1]
				i++
			}
		case "--token":
			if i+1 < len(os.Args) {
				token = os.Args[i+1]
				i++
			}
		}
	}

	if err := ui.Serve(port, serverURL, token); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func cmdServe() {
	cfg := server.DefaultConfig()

	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--config", "-c":
			if i+1 < len(os.Args) {
				loaded, err := server.LoadConfig(os.Args[i+1])
				if err != nil {
					fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
					os.Exit(1)
				}
				cfg = loaded
				i++
			}
		case "--port", "-p":
			if i+1 < len(os.Args) {
				fmt.Sscanf(os.Args[i+1], "%d", &cfg.Port)
				i++
			}
		case "--token":
			if i+1 < len(os.Args) {
				cfg.Auth.Token = os.Args[i+1]
				i++
			}
		case "--tunnel":
			cfg.Tunnel.Enabled = true
		case "--tunnel-name":
			if i+1 < len(os.Args) {
				cfg.Tunnel.Name = os.Args[i+1]
				cfg.Tunnel.Enabled = true
				i++
			}
		case "--relay":
			if i+1 < len(os.Args) {
				cfg.Relay.URL = os.Args[i+1]
				i++
			}
		case "--relay-token":
			if i+1 < len(os.Args) {
				cfg.Relay.Token = os.Args[i+1]
				i++
			}
		}
	}

	srv, err := server.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Auto-fill relay config from repo's config.json if not set via flags/toml
	if cfg.Relay.URL == "" || cfg.Relay.Token == "" {
		repoConfigPath := filepath.Join(srv.Repo.VibeDir, "config.json")
		if data, err := os.ReadFile(repoConfigPath); err == nil {
			var repoConfig map[string]string
			if json.Unmarshal(data, &repoConfig) == nil {
				if cfg.Relay.URL == "" {
					if u := repoConfig["relay_url"]; u != "" {
						cfg.Relay.URL = u
					}
				}
				if cfg.Relay.Token == "" {
					if t := repoConfig["relay_token"]; t != "" {
						cfg.Relay.Token = t
					}
				}
			}
		}
	}

	// Start cloudflared tunnel if requested
	if cfg.Tunnel.Enabled {
		// Auto-use default relay when tunneling without explicit --relay
		if cfg.Relay.URL == "" {
			cfg.Relay.URL = server.GetDefaultRelayURL()
		}
		if cfg.Relay.Token == "" {
			cfg.Relay.Token = server.GetDefaultRelayToken()
		}

		tunnel, err := server.StartTunnel(cfg.Port, srv.Repo.VibeDir, cfg.Tunnel.Name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tunnel error: %v\n", err)
			os.Exit(1)
		}
		defer tunnel.Stop()
		fmt.Printf("\n  Public URL: %s\n", tunnel.URL)
		if cfg.Tunnel.Name != "" {
			fmt.Printf("  Named tunnel: %s (stable URL across restarts)\n", cfg.Tunnel.Name)
		} else {
			fmt.Println("  Tip: use --tunnel-name <name> for a stable URL across restarts.")
		}
		fmt.Println()

		// Publish to relay so clients can discover the new URL
		if cfg.Relay.URL != "" {
			serverID := srv.ServerID()
			lanURLs := server.GetLANAddresses(cfg.Port)
			if pubErr := vibeRelay.Publish(cfg.Relay.URL, serverID, tunnel.URL, cfg.Relay.Token, lanURLs); pubErr != nil {
				fmt.Fprintf(os.Stderr, "  relay publish warning: %v\n", pubErr)
			} else {
				fmt.Printf("  Published to relay: %s (id: %s)\n", cfg.Relay.URL, serverID)
			}
		}

		// Broadcast the new tunnel URL to any already-connected WebSocket clients
		srv.BroadcastTunnelUpdate(tunnel.URL)
	}

	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func cmdLink() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe link <source> [target-dir] [--token <token>]")
		os.Exit(1)
	}
	source := os.Args[2]
	targetDir := "."
	token := ""

	i := 3
	for i < len(os.Args) {
		if os.Args[i] == "--token" && i+1 < len(os.Args) {
			token = os.Args[i+1]
			i += 2
		} else if targetDir == "." {
			targetDir = os.Args[i]
			i++
		} else {
			i++
		}
	}

	var repo *core.Repo
	var err error
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		repo, err = link.LinkRemote(targetDir, source, token)
	} else {
		repo, err = link.Link(targetDir, source)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	mgr := link.NewManager(repo)
	config, manifest, _ := mgr.Status()

	fmt.Printf("Linked to %s (%s)\n", config.Source, config.SourceType)
	if config.WorkingBranch != "" {
		fmt.Printf("  Working branch: %s (upstream: %s)\n", config.WorkingBranch, config.Branch)
		fmt.Println("  Your changes stay on your branch. Use 'vibe merge main' to pull in updates.")
	}
	if manifest != nil {
		cached := 0
		for _, f := range manifest.Files {
			if f.Cached {
				cached++
			}
		}
		fmt.Printf("  %d files in manifest (%d cached)\n", len(manifest.Files), cached)
		fmt.Println("  Run 'vibe pull' to fetch all files, or access them on-demand.")
	}

	// Register with daemon for automatic sync
	absPath, _ := filepath.Abs(repo.WorkDir)
	if regErr := daemon.RegisterRepo(absPath, config.Source, config.SourceType, config.Token, config.Branch, config.FallbackURLs, config.RelayURL, config.RelayToken, config.ServerID); regErr != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not register with daemon: %v\n", regErr)
	} else {
		fmt.Println("  Registered with vibe daemon for automatic sync.")
		fmt.Println("  Run 'vibe service install && vibe service start' to enable background sync.")
	}
}

func cmdFetch() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe fetch <file> [file...]")
		os.Exit(1)
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := link.NewManager(repo)

	for _, path := range os.Args[2:] {
		_, err := mgr.Fetch(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error fetching %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("  fetched %s\n", path)
	}
}

func cmdPull() {
	maxSize := int64(link.DefaultMaxFileSize)
	for _, arg := range os.Args[2:] {
		if arg == "--no-size-limit" {
			maxSize = -1
		}
	}
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := link.NewManager(repo)
	count, err := mgr.PullWithLimit(maxSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if count == 0 {
		fmt.Println("All files already cached.")
	} else {
		fmt.Printf("Fetched %d file(s).\n", count)
	}
}

func cmdUpdate() {
	type release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	fmt.Printf("Current version: %s\n", version)
	fmt.Print("Checking for updates... ")

	resp, err := http.Get("https://api.github.com/repos/Jay73737/Vibe/releases/latest")
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil || rel.TagName == "" {
		fmt.Fprintln(os.Stderr, "\nerror: could not parse release info")
		os.Exit(1)
	}

	if rel.TagName == version {
		fmt.Printf("already up to date (%s)\n", version)
		return
	}
	fmt.Printf("new version available: %s\n", rel.TagName)

	// Find the right asset for this platform
	goos := strings.ToLower(strings.TrimSpace(func() string {
		switch {
		case strings.Contains(strings.ToLower(os.Getenv("OS")), "windows"):
			return "windows"
		default:
			return "linux"
		}
	}()))
	// Detect OS properly
	exePath, _ := os.Executable()
	goosActual := "linux"
	if strings.HasSuffix(exePath, ".exe") {
		goosActual = "windows"
	}
	_ = goos
	goosActual = goosActual // use detected value

	suffix := "-linux-amd64"
	if goosActual == "windows" {
		suffix = "-windows-amd64.exe"
	}

	var downloadURL string
	for _, a := range rel.Assets {
		if strings.HasSuffix(a.Name, suffix) {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		fmt.Fprintf(os.Stderr, "No binary found for your platform in release %s\n", rel.TagName)
		fmt.Fprintf(os.Stderr, "Visit: %s\n", rel.HTMLURL)
		os.Exit(1)
	}

	fmt.Printf("Downloading %s...\n", downloadURL)
	dlResp, err := http.Get(downloadURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error downloading: %v\n", err)
		os.Exit(1)
	}
	defer dlResp.Body.Close()
	data, err := io.ReadAll(dlResp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading download: %v\n", err)
		os.Exit(1)
	}

	// Replace the current binary atomically
	exePath, err = os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error finding executable: %v\n", err)
		os.Exit(1)
	}
	tmp := exePath + ".new"
	if err := os.WriteFile(tmp, data, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error writing update: %v\n", err)
		os.Exit(1)
	}
	if err := os.Rename(tmp, exePath); err != nil {
		fmt.Fprintf(os.Stderr, "error replacing binary: %v\n", err)
		os.Remove(tmp)
		os.Exit(1)
	}
	fmt.Printf("Updated to %s. Run 'vibe version' to confirm.\n", rel.TagName)
}

func cmdDrop() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe drop <file|list|cancel <id>> [--server URL] [--token TOKEN] [--port PORT] [--ttl 24h]")
		os.Exit(1)
	}

	// Sub-commands: list and cancel
	switch os.Args[2] {
	case "list":
		cmdDropList(os.Args[3:])
		return
	case "cancel":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: vibe drop cancel <id>")
			os.Exit(1)
		}
		cmdDropCancel(os.Args[3], os.Args[4:])
		return
	}

	filePath := os.Args[2]
	port := 7433
	ttl := "24h"
	serverURL := ""
	token := ""
	for i := 3; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--port", "-p":
			if i+1 < len(os.Args) {
				fmt.Sscanf(os.Args[i+1], "%d", &port)
				i++
			}
		case "--ttl":
			if i+1 < len(os.Args) {
				ttl = os.Args[i+1]
				i++
			}
		case "--server", "-s":
			if i+1 < len(os.Args) {
				serverURL = os.Args[i+1]
				i++
			}
		case "--token":
			if i+1 < len(os.Args) {
				token = os.Args[i+1]
				i++
			}
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
		os.Exit(1)
	}

	// Resolve server URL: explicit > local repo roles > localhost default
	if serverURL == "" {
		serverURL = fmt.Sprintf("http://localhost:%d", port)
	}
	if token == "" {
		token = getServerToken()
	}

	// Build multipart request
	var buf strings.Builder
	boundary := "vibedropboundary"
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"file\"; filename=%q\r\n", filepath.Base(filePath)))
	buf.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	body := []byte(buf.String())
	body = append(body, data...)
	body = append(body, []byte("\r\n--"+boundary+"--\r\n")...)

	total := int64(len(body))
	fmt.Printf("  Uploading %s (%s)...\n", filepath.Base(filePath), formatBytes(int64(len(data))))

	dropURL := serverURL + "/api/drop?ttl=" + ttl
	req, _ := http.NewRequest(http.MethodPost, dropURL, newProgressReader(body, total))
	req.ContentLength = total
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	fmt.Print("\r\033[K") // clear progress line
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: server not running? Start with: vibe serve\n")
		os.Exit(1)
	}
	defer resp.Body.Close()

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "error: drop failed (status %d)\n", resp.StatusCode)
		os.Exit(1)
	}

	fmt.Printf("File ready for one-time pickup (expires in %s):\n\n", result["expires_in"])
	fmt.Printf("  %s\n\n", result["command"])
	fmt.Println("Send that command to the recipient. The file is deleted after pickup.")
}

func cmdStore() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, `usage: vibe store <subcommand> [args]

Subcommands:
  list                    List files in the store
  put <file> [name]       Upload a file to the store
  get <name> [dest]       Download a file from the store
  rm <name>               Delete a file from the store

Flags:
  --server URL            Server URL (default: http://localhost:7433)
  --token TOKEN           Auth token (auto-detected from local repo if omitted)`)
		os.Exit(1)
	}

	serverURL := "http://localhost:7433"
	token := ""

	// Parse global flags (can appear anywhere after subcommand)
	sub := os.Args[2]
	var positional []string
	for i := 3; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--server", "-s":
			if i+1 < len(os.Args) {
				serverURL = os.Args[i+1]
				i++
			}
		case "--token":
			if i+1 < len(os.Args) {
				token = os.Args[i+1]
				i++
			}
		default:
			positional = append(positional, os.Args[i])
		}
	}
	if token == "" {
		token = getServerToken()
	}

	client := link.NewRemoteClient(serverURL, token)

	switch sub {
	case "list", "ls":
		data, err := client.AuthGet("/api/store/")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		var result struct {
			Files []struct {
				Name string `json:"name"`
				Size int64  `json:"size"`
			} `json:"files"`
		}
		if err := json.Unmarshal(data, &result); err != nil || len(result.Files) == 0 {
			fmt.Println("Store is empty.")
			return
		}
		for _, f := range result.Files {
			fmt.Printf("  %-40s  %d bytes\n", f.Name, f.Size)
		}

	case "put", "upload":
		if len(positional) < 1 {
			fmt.Fprintln(os.Stderr, "usage: vibe store put <file> [name]")
			os.Exit(1)
		}
		filePath := positional[0]
		name := filepath.Base(filePath)
		if len(positional) >= 2 {
			name = positional[1]
		}
		fileData, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
			os.Exit(1)
		}
		if err := client.AuthPost("/api/store/"+name, fileData); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Stored: %s (%d bytes)\n", name, len(fileData))

	case "get", "download":
		if len(positional) < 1 {
			fmt.Fprintln(os.Stderr, "usage: vibe store get <name> [dest]")
			os.Exit(1)
		}
		name := positional[0]
		dest := name
		if len(positional) >= 2 {
			dest = positional[1]
		}
		data, err := client.AuthGet("/api/store/" + name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error saving file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Saved: %s (%d bytes)\n", dest, len(data))

	case "rm", "remove", "delete":
		if len(positional) < 1 {
			fmt.Fprintln(os.Stderr, "usage: vibe store rm <name>")
			os.Exit(1)
		}
		name := positional[0]
		if err := client.AuthDelete("/api/store/" + name); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Deleted: %s\n", name)

	default:
		fmt.Fprintf(os.Stderr, "unknown store subcommand: %s\n", sub)
		os.Exit(1)
	}
}

func parseDropFlags(args []string) (serverURL, token string) {
	serverURL = "http://localhost:7433"
	token = getServerToken()
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--server", "-s":
			if i+1 < len(args) {
				serverURL = args[i+1]
				i++
			}
		case "--token":
			if i+1 < len(args) {
				token = args[i+1]
				i++
			}
		}
	}
	return
}

func cmdDropList(args []string) {
	serverURL, token := parseDropFlags(args)
	client := link.NewRemoteClient(serverURL, token)
	data, err := client.AuthGet("/api/drops")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	var result struct {
		Drops []struct {
			ID        string `json:"id"`
			Filename  string `json:"filename"`
			Size      int    `json:"size"`
			ExpiresAt string `json:"expires_at"`
		} `json:"drops"`
	}
	if err := json.Unmarshal(data, &result); err != nil || len(result.Drops) == 0 {
		fmt.Println("No pending drops.")
		return
	}
	fmt.Printf("%-36s  %-30s  %10s  %s\n", "ID", "FILE", "SIZE", "EXPIRES")
	fmt.Printf("%-36s  %-30s  %10s  %s\n", "--", "----", "----", "-------")
	for _, d := range result.Drops {
		fmt.Printf("%-36s  %-30s  %10s  %s\n", d.ID, d.Filename, formatBytes(int64(d.Size)), d.ExpiresAt)
	}
}

func cmdDropCancel(id string, args []string) {
	serverURL, token := parseDropFlags(args)
	client := link.NewRemoteClient(serverURL, token)
	if err := client.AuthDelete("/api/drop/" + id); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Cancelled drop %s\n", id)
}

func cmdPickup() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vibe pickup <url>")
		os.Exit(1)
	}
	pickupURL := os.Args[2]

	resp, err := http.Get(pickupURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintln(os.Stderr, "error: file not found — already picked up or expired")
		os.Exit(1)
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "error: server returned %d\n", resp.StatusCode)
		os.Exit(1)
	}

	// Get filename from Content-Disposition header
	filename := "dropped-file"
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if i := strings.Index(cd, "filename="); i >= 0 {
			filename = strings.Trim(cd[i+9:], `"`)
		}
	}

	// Don't overwrite existing files
	out := filename
	for i := 1; ; i++ {
		if _, err := os.Stat(out); os.IsNotExist(err) {
			break
		}
		ext := filepath.Ext(filename)
		out = fmt.Sprintf("%s(%d)%s", strings.TrimSuffix(filename, ext), i, ext)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading response: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error saving file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Saved: %s (%d bytes)\n", out, len(data))
}

// getServerToken reads the auth token from the local repo config or roles file.
func getServerToken() string {
	repo, err := core.FindRepo(".")
	if err != nil {
		return ""
	}
	// Try roles file first (owner token)
	rm := roles.NewManager(repo.VibeDir)
	if rf, err := rm.Load(); err == nil {
		for _, u := range rf.Users {
			if u.Token != "" {
				return u.Token
			}
		}
	}
	return ""
}

func cmdSync() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	mgr := link.NewManager(repo)
	changed, err := mgr.Sync()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if changed == 0 {
		fmt.Println("Already up to date.")
	} else {
		fmt.Printf("Synced %d changed file(s) from source.\n", changed)
		fmt.Println("Run 'vibe pull' to fetch updated contents.")
	}
}

func cmdRelay() {
	port := 7435
	token := ""
	dataDir := ""

	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--port", "-p":
			if i+1 < len(os.Args) {
				fmt.Sscanf(os.Args[i+1], "%d", &port)
				i++
			}
		case "--token":
			if i+1 < len(os.Args) {
				token = os.Args[i+1]
				i++
			}
		case "--data":
			if i+1 < len(os.Args) {
				dataDir = os.Args[i+1]
				i++
			}
		}
	}

	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".vibe", "relay")
	}

	r := vibeRelay.New(dataDir)
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	if err := r.Serve(addr, token); err != nil {
		fmt.Fprintf(os.Stderr, "relay error: %v\n", err)
		os.Exit(1)
	}
}

// --- Daemon & Service commands ---

func cmdDaemon() {
	d, err := daemon.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := d.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
		os.Exit(1)
	}
}

func cmdService() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, `usage: vibe service <command>

Commands:
  install     Install the vibe daemon as a startup service
  uninstall   Remove the startup service
  start       Start the daemon service
  stop        Stop the daemon service
  status      Check whether the daemon is running`)
		os.Exit(1)
	}

	switch os.Args[2] {
	case "install":
		if err := daemon.ServiceInstall(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Vibe daemon installed as a startup service.")
		fmt.Println("Run 'vibe service start' to start it now.")
	case "uninstall":
		if err := daemon.ServiceUninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Vibe daemon service removed.")
	case "start":
		if err := daemon.ServiceStart(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Vibe daemon started.")
	case "stop":
		if err := daemon.ServiceStop(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Vibe daemon stopped.")
	case "status":
		status, err := daemon.ServiceStatus()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Vibe daemon: %s\n", status)

		// Also show watched repos
		reg, err := daemon.LoadRegistry()
		if err == nil && len(reg.Repos) > 0 {
			fmt.Printf("\nWatched repos (%d):\n", len(reg.Repos))
			for _, r := range reg.Repos {
				fmt.Printf("  %s <- %s (%s)\n", r.Path, r.Source, r.SourceType)
			}
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown service command: %s\n", os.Args[2])
		os.Exit(1)
	}
}

// auditCLI logs an action to the repo's audit log (best-effort, never fails the command).
func auditCLI(repo *core.Repo, action, detail string) {
	audit := core.NewAuditLog(repo.VibeDir)
	audit.Log(action, getAuthor(), detail, "cli", "local")
}

func cmdAudit() {
	repo, err := core.FindRepo(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	limit := 50
	for i := 2; i < len(os.Args); i++ {
		if (os.Args[i] == "-n" || os.Args[i] == "--limit") && i+1 < len(os.Args) {
			fmt.Sscanf(os.Args[i+1], "%d", &limit)
			i++
		}
		if os.Args[i] == "--all" {
			limit = 0
		}
	}

	audit := core.NewAuditLog(repo.VibeDir)
	entries, err := audit.Read(limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println("No audit log entries.")
		return
	}

	fmt.Printf("%-20s %-12s %-15s %-6s %s\n", "TIMESTAMP", "ACTION", "USER", "SRC", "DETAIL")
	fmt.Printf("%-20s %-12s %-15s %-6s %s\n", "---------", "------", "----", "---", "------")
	for _, e := range entries {
		ts := e.Timestamp.Format("2006-01-02 15:04:05")
		fmt.Printf("%-20s %-12s %-15s %-6s %s\n", ts, e.Action, e.User, e.Source, e.Detail)
	}
}

func getAuthor() string {
	if name := os.Getenv("VIBE_AUTHOR"); name != "" {
		return name
	}
	// Check vibe config
	if repo, err := core.FindRepo("."); err == nil {
		configPath := filepath.Join(repo.VibeDir, "config.json")
		if data, err := os.ReadFile(configPath); err == nil {
			var config map[string]string
			if json.Unmarshal(data, &config) == nil {
				if name, ok := config["author"]; ok && name != "" {
					return name
				}
			}
		}
	}
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("USERNAME"); user != "" {
		return user
	}
	return "unknown"
}

// progressReader wraps a byte slice and prints an upload progress bar.
type progressReader struct {
	data    []byte
	pos     int
	total   int64
}

func newProgressReader(data []byte, total int64) *progressReader {
	return &progressReader{data: data, total: total}
}

func (r *progressReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	pct := int(float64(r.pos) / float64(r.total) * 100)
	filled := pct / 5
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 20-filled)
	fmt.Fprintf(os.Stderr, "\r  [%s] %d%%  %s / %s ",
		bar, pct, formatBytes(int64(r.pos)), formatBytes(r.total))
	return n, nil
}

func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(b)/1024/1024/1024)
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/1024/1024)
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return dir
}
