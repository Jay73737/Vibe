# Vibe

A modern, lightweight version control system built for the vibe coding era. Spin up branches instantly, experiment freely, nuke what doesn't work, and share your vibes with your team — all from a single binary under 15MB.

## Why Vibe?

- **Vibe coding workflow.** `vibe vibe experiment` spins up a branch. `vibe save` commits everything. `vibe nuke` throws it away. No ceremony.
- **Link, don't clone.** `vibe link` connects to any repo — metadata syncs instantly, files fetch on-demand.
- **Sessions, not stashes.** Switch branches and your work is auto-saved. Come back anytime with `vibe restore`.
- **Built-in collaboration.** User roles (admin, contributor, reader) and per-user tokens are first-class.
- **Host anywhere.** `vibe serve` runs on your laptop, a Raspberry Pi, a VPS, or Docker. Lightweight enough for any machine.
- **Real-time push.** Connected users get live updates over WebSocket when anyone pushes changes.
- **CLI + Web UI.** Power users get a full CLI. Everyone else gets `vibe ui`.

## Quick Start

### Install from source

```bash
git clone https://github.com/Jay73737/Vibe.git
cd Vibe
go build -o vibe ./cmd/vibe/
```

Move the binary to your PATH:

```bash
# Linux/macOS
sudo mv vibe /usr/local/bin/

# Windows — move vibe.exe to a folder in your PATH
```

### The vibe coding workflow

```bash
# Set up
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

### Import from Git

```bash
vibe import https://github.com/someone/repo.git
cd repo
vibe log
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
| `vibe fetch <file>` | Fetch a single file from source on-demand |
| `vibe pull` | Fetch all files from source |
| `vibe sync` | Pull latest changes from source |
| `vibe import <git-url>` | Clone a git repo and convert to Vibe |

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

### Server

| Command | Description |
|---------|-------------|
| `vibe serve` | Start the Vibe server (default port 7433) |
| `vibe serve --port 8080` | Custom port |
| `vibe serve --token mysecret` | Require auth token |
| `vibe serve --config server.toml` | Use a config file |
| `vibe ui` | Launch the web UI (default port 7434) |
| `vibe ui --server http://host:7433` | Point UI at a remote server |

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

### Quick start (local)

```bash
cd my-project
vibe init
vibe config author "yourname"
vibe save "first commit"
vibe roles init yourname
vibe serve --port 7433
```

Others connect with:

```bash
vibe link http://your-ip:7433 my-copy --token <their-token>
vibe pull
```

### Config file

Copy `configs/vibe-server.toml` and customize:

```toml
host = "0.0.0.0"
port = 7433
repo_path = "/path/to/repo"

[tls]
enabled = true
cert_file = "/etc/ssl/cert.pem"
key_file = "/etc/ssl/key.pem"

[auth]
token = "shared-secret"
```

### Docker

```bash
docker build -t vibe-server .
docker run -p 7433:7433 -v /path/to/repo:/repo vibe-server serve --port 7433
```

### systemd

Copy `configs/vibe.service` to `/etc/systemd/system/`:

```bash
sudo systemctl enable vibe
sudo systemctl start vibe
```

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
├── config.json       # Local config (author, etc.)
├── roles.json        # User roles and tokens
├── link.json         # Link source config
├── manifest.json     # Linked file manifest
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
