# Vibe

A modern, lightweight version control system built for the vibe coding era. Spin up branches instantly, experiment freely, nuke what doesn't work, and share your work with your team — all from a single binary.

## Why Vibe?

- **Vibe coding workflow.** `vibe vibe experiment` spins up a branch. `vibe save` commits everything. `vibe nuke` throws it away. No ceremony.
- **Link, don't clone.** `vibe link` connects to any repo — metadata syncs instantly, files fetch on-demand.
- **Sessions, not stashes.** Switch branches and your work is auto-saved. Come back anytime with `vibe restore`.
- **Built-in collaboration.** User roles (admin, contributor, reader) and per-user tokens are first-class.
- **Host anywhere.** `vibe serve` runs on your laptop, a Raspberry Pi, a VPS, or any machine.
- **Tunnel from anywhere.** `vibe serve --tunnel` exposes your repo to the internet instantly via Cloudflare — no port forwarding, no static IP required.
- **Auto-reconnect.** If the server restarts and gets a new tunnel URL, connected clients re-discover it automatically. No manual re-linking.
- **Background sync.** A lightweight daemon runs at startup and pulls changes automatically — no manual `vibe sync` needed.
- **Real-time updates.** Connected users get live notifications over WebSocket when changes are pushed.
- **File transfer.** One-time drop links and a persistent file store, separate from version control.
- **CLI + Web UI.** Full CLI for power users, `vibe ui` for everyone else.

---

## Install

### Linux / macOS

```bash
curl -fsSL https://raw.githubusercontent.com/Jay73737/Vibe/main/install.sh | bash
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/Jay73737/Vibe/main/install.ps1 | iex
```

The installer handles everything: installs Go if needed, installs cloudflared for tunnel support, builds the `vibe` binary, adds it to your PATH, and starts the background daemon service.

Open a **new terminal** after installing, then run `vibe help`.

### Build from source

```bash
git clone https://github.com/Jay73737/Vibe.git
cd Vibe
go build -o vibe ./cmd/vibe/
sudo mv vibe /usr/local/bin/     # Linux/macOS
# Windows: move vibe.exe to a folder in your PATH
```

### Update

```bash
vibe update
```

Downloads the latest release binary for your platform and replaces the current executable. Or re-run the installer above — it updates cleanly on all platforms.

---

## Quick Start

```bash
mkdir my-project && cd my-project
vibe init
vibe config author "Your Name"

# Save some files
echo "hello world" > hello.txt
vibe save "initial commit"

# Spin up an experiment branch
vibe vibe my-experiment

# Hack away...
echo "wild idea" > idea.txt
vibe save "trying something"

# Didn't pan out? Nuke it and go back to main.
vibe nuke

# Worked? Merge it in.
vibe switch main
vibe merge my-experiment
```

---

## Commands

### Quick Workflow

| Command | Description |
|---------|-------------|
| `vibe vibe <name>` | Create a branch and switch to it instantly |
| `vibe save [message]` | Stage all files and commit in one shot |
| `vibe nuke [name]` | Destroy a branch and switch back to main |
| `vibe share <branch> <user>` | Notify a connected user about a branch |

### Core

| Command | Description |
|---------|-------------|
| `vibe init [dir]` | Create a new repository |
| `vibe uninit` | Delete the repo, broadcast shutdown to clients, clean up relay |
| `vibe add <files>` | Stage files (`vibe add .` for all) |
| `vibe commit -m "msg"` | Commit staged files |
| `vibe status` | Show staged, modified, and untracked files |
| `vibe log` | Show commit history |
| `vibe config author "Name"` | Set your author name |
| `vibe version` | Show installed version |
| `vibe update` | Update to the latest release |

### Branching & Sessions

| Command | Description |
|---------|-------------|
| `vibe branch <name>` | Create a new branch |
| `vibe branches` | List all branches with lineage |
| `vibe switch <name>` | Switch branch (auto-saves work as a session) |
| `vibe switch <name> --no-session` | Switch without saving |
| `vibe merge <branch>` | Merge a branch into current (three-way merge) |
| `vibe destroy <name>` | Delete a branch and its sessions |
| `vibe sessions` | List all saved sessions |
| `vibe restore <session-id>` | Restore a saved session |

### Version History

| Command | Description |
|---------|-------------|
| `vibe diff` | Diff working tree vs last commit |
| `vibe diff <hash1> <hash2>` | Diff between two commits |
| `vibe revert <hash>` | Revert to a previous commit (creates a revert commit) |
| `vibe blame <file>` | Per-line authorship |

### Linking & Sync

Linking connects your local directory to a remote vibe server. Files are fetched on-demand and cached locally. The daemon keeps you in sync automatically.

| Command | Description |
|---------|-------------|
| `vibe link <url> [dir] --token <t>` | Link to a remote server |
| `vibe link <path> [dir]` | Link to a local repo |
| `vibe fetch <file>` | Fetch a single file on-demand |
| `vibe pull` | Fetch all files (skips files >100MB by default) |
| `vibe pull --no-size-limit` | Fetch all files including large ones |
| `vibe sync` | Pull latest refs from source (one-shot) |
| `vibe import <git-url>` | Import a git repo and convert to Vibe |

### File Transfer

One-time drops let you send a file to anyone via a single-use link — the file is deleted from the server immediately after pickup. The store is a persistent scratch space for files you want to share without committing them to version control.

| Command | Description |
|---------|-------------|
| `vibe drop <file>` | Create a one-time pickup link (24h TTL) |
| `vibe drop <file> --ttl 1h` | Custom expiry time |
| `vibe drop <file> --server <url> --token <t>` | Drop to a remote server (no local repo needed) |
| `vibe drop list` | List all pending drops |
| `vibe drop cancel <id>` | Cancel a drop before it's picked up |
| `vibe pickup <url>` | Download a one-time drop link |
| `vibe store list` | List files in the persistent store |
| `vibe store put <file> [name]` | Upload a file to the store |
| `vibe store get <name> [dest]` | Download a file from the store |
| `vibe store rm <name>` | Delete a file from the store |

Store files live in `.vibe/store/` on the server — not committed, not synced, not version-controlled. All store commands accept `--server <url>` and `--token <t>` to target a remote server without a local vibe repo.

### Roles & Permissions

| Command | Description |
|---------|-------------|
| `vibe roles init <name>` | Initialize roles (you become admin) |
| `vibe roles` | List all users and their roles |
| `vibe grant <user> <role>` | Assign a role: `admin`, `contributor`, or `reader` |
| `vibe revoke <user>` | Remove a user's access |
| `vibe invite <user> [role]` | Generate a ready-to-paste `vibe link` command for a user |

**Roles:**
- **Admin** — Full control. Manage users, push, configure.
- **Contributor** — Create branches and push changes.
- **Reader** — Read-only. Can link and pull, but not push.

### Server & Tunnels

| Command | Description |
|---------|-------------|
| `vibe serve` | Start the server (default port 7433) |
| `vibe serve --tunnel` | Expose to the internet via Cloudflare tunnel |
| `vibe serve --tunnel-name <name>` | Named tunnel — stable URL across restarts |
| `vibe serve --port 8080` | Custom port |
| `vibe serve --token <secret>` | Require an auth token |
| `vibe serve --config vibe.toml` | Load config from a file |
| `vibe ui` | Open the web dashboard (default port 7434) |
| `vibe ui --server <url> --token <t>` | Connect UI to a remote server |

### Daemon & Service

The daemon runs in the background and automatically syncs all linked repos. It re-discovers servers after restarts using stored fallback addresses and the relay.

| Command | Description |
|---------|-------------|
| `vibe daemon` | Run the sync daemon in the foreground (for debugging) |
| `vibe service install` | Install daemon as a startup service |
| `vibe service uninstall` | Remove the startup service |
| `vibe service start` | Start the service |
| `vibe service stop` | Stop the service |
| `vibe service status` | Check service status and watched repos |

### Security & Audit

| Command | Description |
|---------|-------------|
| `vibe audit` | View the last 50 audit log entries |
| `vibe audit -n 100` | View more entries |
| `vibe audit --all` | View full audit log |

### Relay (Self-hosted)

A built-in relay is already configured — you don't need to run your own. If you want to self-host:

| Command | Description |
|---------|-------------|
| `vibe relay` | Run a relay server (default port 7435) |
| `vibe relay --port 8080` | Custom port |
| `vibe relay --data /path` | Persist relay entries to disk |

---

## Sharing a Repo with Others

### Full walkthrough

**On the host machine:**

```bash
# 1. Set up your repo
vibe init
vibe config author "yourname"
vibe save "first commit"

# 2. Set up roles (makes you admin)
vibe roles init yourname

# 3. Start the server with a public tunnel
vibe serve --tunnel
```

**In a second terminal (still on host):**

```bash
# 4. Invite a user — prints a ready-to-run command for them
vibe invite Alice contributor
# Output: vibe link https://xyz.trycloudflare.com --token abc123...
```

**On Alice's machine:**

```bash
# 5. Alice runs the printed command
vibe link https://xyz.trycloudflare.com --token abc123...

# 6. Pull all files
vibe pull
```

Alice is now connected. The daemon keeps her in sync automatically. If your server restarts and gets a new tunnel URL, Alice's daemon re-discovers it — no re-linking needed.

### Local network only

```bash
# Host
vibe serve

# Client (replace with actual IP)
vibe link http://192.168.1.10:7433 my-copy --token <token>
vibe pull
```

---

## Config File

Use a config file instead of flags:

```toml
host = "0.0.0.0"
port = 7433
repo_path = "/path/to/repo"

[auth]
token = "your-secret-token"

[tunnel]
enabled = true

[relay]
url   = "https://vibe-relay.cky37373.workers.dev"
token = ""  # leave empty — auto-generated per repo at vibe init
```

Start with: `vibe serve --config vibe.toml`

A full example is in [`configs/vibe.toml.example`](configs/vibe.toml.example).

---

## .vibeignore

Exclude files from tracking, like `.gitignore`:

```
node_modules
*.log
.env
dist
.DS_Store
```

---

## How Tunnel Re-discovery Works

When a server restarts it gets a new random tunnel URL. Here's how clients stay connected automatically:

1. **LAN fallback** — At link time, the client stores the server's LAN IP addresses. If the tunnel changes but they're on the same network, the daemon reconnects directly.
2. **Relay lookup** — Each repo has a unique token generated at `vibe init`. When the server starts with `--tunnel`, it publishes its new URL to the relay (authenticated with that token). The daemon queries the relay when all known URLs fail.
3. **WebSocket reconnect** — Once the daemon finds the new URL, it reconnects and resumes live sync.

No manual intervention required.

---

## Web UI

`vibe ui` opens a browser dashboard at `http://localhost:7434` showing:

- Repository info (branch, HEAD, file count)
- File browser with on-demand fetch
- Commit history
- Branch list
- User roles
- Live event feed (real-time WebSocket)

---

## How It Works

Vibe uses a content-addressable object store with SHA-256 hashing. Every file, tree, and commit is an immutable object stored by hash — similar to Git's internals.

```
.vibe/
├── HEAD              # Current branch pointer
├── index             # Staging area
├── config.json       # Local config (author, relay URL, relay token)
├── roles.json        # User roles and per-user tokens
├── link.json         # Link config (source URL, relay token, server ID)
├── manifest.json     # File manifest for linked repos
├── audit.log         # Audit log
├── store/            # Persistent file store (not VCS-tracked)
├── objects/          # Content-addressable store (blobs, trees, commits)
└── refs/
    ├── branches/     # Branch head pointers
    └── sessions/     # Auto-saved session snapshots
```

Linking uses hybrid sync: the file manifest (names, hashes, sizes) syncs immediately, actual file contents are fetched on-demand and cached locally.

---

## Cross-Platform Builds

```bash
GOOS=linux   GOARCH=amd64 go build -o vibe-linux-amd64   ./cmd/vibe/
GOOS=linux   GOARCH=arm64 go build -o vibe-linux-arm64   ./cmd/vibe/
GOOS=darwin  GOARCH=amd64 go build -o vibe-darwin-amd64  ./cmd/vibe/
GOOS=darwin  GOARCH=arm64 go build -o vibe-darwin-arm64  ./cmd/vibe/
GOOS=windows GOARCH=amd64 go build -o vibe-windows-amd64.exe ./cmd/vibe/
```

Release builds are created automatically via GitHub Actions when a version tag is pushed (`git tag v1.0.0 && git push origin v1.0.0`).

---

## Contributing

1. Fork the repo
2. `vibe vibe my-feature` (or `git checkout -b my-feature`)
3. Make changes
4. `go test ./...`
5. Submit a PR

---

## License

MIT License. See [LICENSE](LICENSE) for details.
