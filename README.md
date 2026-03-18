# Vibe

A modern, lightweight version control system built for the vibe coding era. Spin up branches instantly, experiment freely, nuke what doesn't work, and share your vibes with your team — all from a single binary under 15MB.

## Why Vibe?

- **Vibe coding workflow.** `vibe vibe experiment` spins up a branch. `vibe save` commits everything. `vibe nuke` throws it away. No ceremony.
- **Link, don't clone.** `vibe link` connects to any repo — metadata syncs instantly, files fetch on-demand.
- **Sessions, not stashes.** Switch branches and your work is auto-saved. Come back anytime with `vibe restore`.
- **Built-in collaboration.** User roles (admin, contributor, reader) and per-user tokens are first-class.
- **Host anywhere.** `vibe serve` runs on your laptop, a Raspberry Pi, a VPS, or Docker. Lightweight enough for any machine.
- **Tunnel from anywhere.** `vibe serve --tunnel` exposes your repo to the internet instantly via Cloudflare — no port forwarding, no static IP.
- **Auto-reconnect.** If the server restarts and gets a new tunnel URL, connected clients discover it automatically via a relay server. No manual re-linking.
- **Background sync.** A lightweight daemon runs at startup and automatically pulls changes whenever the source pushes — no manual `vibe sync` needed.
- **Real-time push.** Connected users get live updates over WebSocket when anyone pushes changes.
- **CLI + Web UI.** Power users get a full CLI. Everyone else gets `vibe ui`.

## Install

### One-line install (Linux/macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/Jay73737/Vibe/main/install.sh | bash
```

### One-line install (Windows)

```powershell
irm https://raw.githubusercontent.com/Jay73737/Vibe/main/install.ps1 | iex
```

The installer:
- Installs Go if not present
- Installs cloudflared for tunnel support
- Builds and installs the `vibe` binary
- Installs and starts the background daemon service

### Install from source

```bash
git clone https://github.com/Jay73737/Vibe.git
cd Vibe
go build -o vibe ./cmd/vibe/
sudo mv vibe /usr/local/bin/   # Linux/macOS
# Windows: move vibe.exe to a folder in your PATH
```

## Quick Start

```bash
mkdir my-project && cd my-project
vibe init
vibe config author "Your Name"

# Create some files and save
echo "hello world" > hello.txt
vibe save "initial setup"

# Start a vibe session — spin up a branch and switch to it
vibe vibe experiment

# Hack away...
echo "wild idea" > experiment.txt
vibe save "trying something"

# Didn't work out? Nuke it.
vibe nuke

# Worked out? Switch back and keep it.
vibe switch main
```

## Commands

### Quick Workflow

| Command | Description |
|---------|-------------|
| `vibe vibe <name>` | Create a branch and switch to it in one shot |
| `vibe save [message]` | Add all files + commit (like git add -A && git commit) |
| `vibe nuke [name]` | Destroy a branch, switch back to main |
| `vibe share <branch> <user>` | Share a branch with a connected user |

### Core

| Command | Description |
|---------|-------------|
| `vibe init [dir]` | Create a new repository |
| `vibe uninit` | Delete the repo, notify clients, clean up relay entry |
| `vibe add <files>` | Stage files (`vibe add .` for all) |
| `vibe commit -m "msg"` | Commit staged files |
| `vibe status` | Show staged, modified, and untracked files |
| `vibe log` | Show commit history |
| `vibe config author "name"` | Set your author name |

### Branching & Sessions

| Command | Description |
|---------|-------------|
| `vibe branch <name>` | Create a new branch |
| `vibe branches` | List all branches |
| `vibe switch <name>` | Switch branch (auto-saves your work as a session) |
| `vibe switch <name> --no-session` | Switch without saving |
| `vibe destroy <name>` | Delete a branch and its sessions |
| `vibe sessions` | List all saved sessions |
| `vibe restore <session-id>` | Restore a saved session |

### Version History

| Command | Description |
|---------|-------------|
| `vibe diff` | Diff working tree vs last commit |
| `vibe diff <hash1> <hash2>` | Diff between two commits |
| `vibe revert <hash>` | Revert to any previous commit (creates a revert commit) |
| `vibe blame <file>` | Per-line authorship |

### Linking & Sync

| Command | Description |
|---------|-------------|
| `vibe link <source> [dir]` | Link to a repo (local path or URL) |
| `vibe link <url> [dir] --token <t>` | Link to a remote server with auth |
| `vibe fetch <file>` | Fetch a single file on-demand |
| `vibe pull` | Fetch all files from source (skips files >100MB by default) |
| `vibe pull --no-size-limit` | Fetch all files including large ones |
| `vibe sync` | Pull latest refs from source |
| `vibe import <git-url>` | Clone a git repo and convert to Vibe |

### File Transfer

One-time drops let you send a file to anyone via a link. The file is deleted from the server after pickup — no trace left behind. The store is a persistent scratch space for files you want to share without committing them to the repo.

| Command | Description |
|---------|-------------|
| `vibe drop <file>` | Create a one-time pickup link (24h TTL by default) |
| `vibe drop <file> --server <url> --token <t>` | Drop to a remote server without a local repo |
| `vibe drop <file> --ttl 1h` | Custom expiry time |
| `vibe pickup <url>` | Download a one-time drop (deleted from server after) |
| `vibe store list` | List files in the persistent store |
| `vibe store put <file> [name]` | Upload a file to the store |
| `vibe store get <name> [dest]` | Download a file from the store |
| `vibe store rm <name>` | Delete a file from the store |

Store files live in `.vibe/store/` on the server — not committed, not synced, not version-controlled. Use `--server` and `--token` on any store command to target a remote server without a local vibe repo.

### Roles & Permissions

| Command | Description |
|---------|-------------|
| `vibe roles init <name>` | Initialize roles (you become admin) |
| `vibe roles` | List all users and roles |
| `vibe grant <user> <role>` | Assign role: `admin`, `contributor`, or `reader` |
| `vibe revoke <user>` | Remove a user's access |

**Roles:**
- **Admin** — Full control. Manage users, push, configure.
- **Contributor** — Create branches, push changes.
- **Reader** — Read-only. Can link and browse, but not modify.

### Server & Tunnels

| Command | Description |
|---------|-------------|
| `vibe serve` | Start the server (default port 7433) |
| `vibe serve --tunnel` | Expose to the internet via Cloudflare tunnel |
| `vibe serve --tunnel-name <name>` | Use a named tunnel for a stable URL across restarts |
| `vibe serve --port 8080` | Custom port |
| `vibe serve --token mysecret` | Require auth token |
| `vibe serve --config server.toml` | Use a config file |
| `vibe invite <user> [role]` | Generate a ready-to-paste link command for a user |
| `vibe ui` | Launch the web UI (default port 7434) |

### Daemon & Service

| Command | Description |
|---------|-------------|
| `vibe daemon` | Run the background sync daemon in the foreground |
| `vibe service install` | Install daemon as a startup service |
| `vibe service uninstall` | Remove the startup service |
| `vibe service start` | Start the service now |
| `vibe service stop` | Stop the service |
| `vibe service status` | Check service status |

The daemon watches all linked repos and automatically pulls changes when the source pushes. If the server restarts with a new tunnel URL, the daemon re-discovers it using stored fallback addresses and the relay server — no manual re-linking required.

### Relay

| Command | Description |
|---------|-------------|
| `vibe relay` | Run a self-hosted relay server (default port 7435) |
| `vibe relay --port 8080` | Custom port |
| `vibe relay --data /path` | Persist relay entries to disk |

The relay is a lightweight key-value server that maps repo server IDs to their current tunnel URLs. When a server restarts and gets a new tunnel URL, it publishes the update to the relay. Daemons on client machines query the relay when they can't reach the last known URL.

A hosted relay is built into the default vibe binary. You can also self-host one anywhere with a stable address.

## .vibeignore

Create a `.vibeignore` file to exclude files from tracking (like `.gitignore`):

```
node_modules
*.log
.env
dist
.DS_Store
```

## Hosting a Vibe Server

Vibe servers are lightweight — they run on anything with a network connection. A Raspberry Pi, an old laptop, a $5 VPS, or localhost.

### Quick start with tunnel (recommended)

```bash
cd my-project
vibe init
vibe config author "yourname"
vibe save "first commit"
vibe roles init yourname
vibe serve --tunnel
```

In another terminal, invite users:

```bash
vibe invite Alice contributor
# Prints: vibe link https://your-tunnel.trycloudflare.com --token abc123...
# Send that command to Alice — she's connected instantly.
```

If the server restarts (e.g. machine reboots), Alice's daemon re-discovers the new tunnel URL automatically via the relay. No re-linking needed.

### Local network only

```bash
vibe serve --port 7433
```

Others connect with:

```bash
vibe link http://your-ip:7433 my-copy --token <their-token>
vibe pull
```

### Config file

```toml
host = "0.0.0.0"
port = 7433
repo_path = "/path/to/repo"

[tls]
enabled = true
cert_file = "/etc/ssl/cert.pem"
key_file  = "/etc/ssl/key.pem"

[auth]
token = "shared-secret"

[tunnel]
enabled = true

[relay]
url   = "https://your-relay.workers.dev"
token = ""  # leave empty — per-repo token is auto-generated at vibe init
```

### Docker

```bash
docker build -t vibe-server .
docker run -p 7433:7433 -v /path/to/repo:/repo vibe-server serve --tunnel
```

### Systemd (Linux)

Copy `configs/vibe-daemon.service` to `~/.config/systemd/user/`:

```bash
systemctl --user enable vibe-daemon
systemctl --user start vibe-daemon
```

Or use `vibe service install` which does this automatically.

## Updating Vibe

### On the machine running this repo

```bash
vibe update
```

This checks GitHub for the latest release, downloads the right binary for your platform, and replaces the current `vibe` executable atomically.

### On a different machine (linked client)

If the other machine was set up with the install script, just run:

```bash
vibe update
```

If it was built from source:

```bash
git pull
go build -o vibe ./cmd/vibe/
sudo mv vibe /usr/local/bin/   # Linux/macOS
# Windows: replace vibe.exe in your PATH folder
```

Or re-run the one-line installer — it pulls the latest and reinstalls:

```bash
# Linux/macOS
curl -fsSL https://raw.githubusercontent.com/Jay73737/Vibe/main/install.sh | bash

# Windows
irm https://raw.githubusercontent.com/Jay73737/Vibe/main/install.ps1 | iex
```

## How Tunnel Re-discovery Works

When a server restarts it gets a new random tunnel URL. Here's how clients stay connected automatically:

1. **LAN fallback** — At link time, the client stores the server's LAN IP addresses. If the tunnel changes but they're on the same network, the daemon reconnects directly.
2. **Relay lookup** — Each repo has a unique token generated at `vibe init`. When the server starts with `--tunnel`, it publishes `{server_id, tunnel_url}` to the relay (authenticated with the per-repo token). The daemon queries the relay when all known URLs fail.
3. **WebSocket reconnect** — Once the daemon finds the new URL, it reconnects and resumes live sync.

No manual intervention required on the client side.

## Web UI

`vibe ui` opens a browser dashboard showing:

- Repository info (branch, HEAD, file count)
- File browser
- Commit history
- Branch list
- User roles
- Live event feed (real-time WebSocket notifications)

## How It Works

Vibe uses a content-addressable object store with SHA-256 hashing. Every file, tree, and commit is an immutable object.

```
.vibe/
├── HEAD              # Current branch pointer
├── index             # Staging area
├── config.json       # Local config (author, relay URL, relay token)
├── roles.json        # User roles and tokens
├── link.json         # Link source config (includes relay token + server ID)
├── manifest.json     # Linked file manifest
├── store/            # Persistent file storage (not VCS-tracked, served at /api/store/)
├── objects/          # Content-addressable store (blobs, trees, commits)
└── refs/
    ├── branches/     # Branch pointers
    └── sessions/     # Auto-saved session snapshots
```

**Linking** uses hybrid sync: directory structure syncs immediately, file contents fetch on-demand and get cached locally.

## Cross-Platform Builds

Single static binary, no dependencies:

```bash
GOOS=linux   GOARCH=amd64 go build -o vibe-linux   ./cmd/vibe/
GOOS=darwin  GOARCH=amd64 go build -o vibe-mac     ./cmd/vibe/
GOOS=windows GOARCH=amd64 go build -o vibe.exe     ./cmd/vibe/
GOOS=linux   GOARCH=arm64 go build -o vibe-arm64   ./cmd/vibe/
```

## Contributing

1. Fork the repo
2. `vibe vibe my-feature` (or `git checkout -b my-feature`)
3. Make changes
4. `go test ./...`
5. Submit a PR

## License

MIT License. See [LICENSE](LICENSE) for details.
