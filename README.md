# Vibe

A modern, developer-friendly version control system built in Go. Vibe rethinks the Git workflow with instant repo linking, auto-saved sessions, built-in user roles, and a real-time web UI — all in a single binary.

## Why Vibe?

- **Link, don't clone.** Connect to any repo with `vibe link` — metadata syncs instantly, files fetch on-demand.
- **Sessions, not stashes.** Switch branches and your work is automatically saved. Come back anytime with `vibe restore`.
- **Built-in collaboration.** User roles (admin, contributor, reader) and token auth are first-class, not bolted on.
- **Self-hosted in seconds.** `vibe serve` runs anywhere — your laptop, a VPS, a Docker container.
- **Real-time push.** Connected clients get live updates over WebSocket when anyone pushes changes.
- **CLI + Web UI.** Power users get a full CLI. Everyone else gets `vibe ui` — a browser-based dashboard.

## Quick Start

### Install from source

```bash
# Requires Go 1.21+
git clone https://github.com/Jay73737/Vibe.git
cd Vibe
go build -o vibe ./cmd/vibe/
```

Move the `vibe` binary somewhere on your `$PATH`:

```bash
# Linux/macOS
sudo mv vibe /usr/local/bin/

# Windows — move vibe.exe to a directory in your PATH
```

### Create a repository

```bash
mkdir my-project && cd my-project
vibe init
```

### Basic workflow

```bash
# Create files
echo "hello world" > hello.txt

# Stage and commit
vibe add hello.txt
vibe commit -m "Initial commit"

# Check status and history
vibe status
vibe log
```

## Commands

### Core

| Command | Description |
|---------|-------------|
| `vibe init [dir]` | Create a new repository |
| `vibe add <files>` | Stage files (`vibe add .` for all) |
| `vibe commit -m "msg"` | Commit staged files |
| `vibe status` | Show staged, modified, and untracked files |
| `vibe log` | Show commit history |

### Branching & Sessions

| Command | Description |
|---------|-------------|
| `vibe branch <name>` | Create a new branch |
| `vibe branches` | List all branches |
| `vibe switch <name>` | Switch branch (auto-saves current work as a session) |
| `vibe switch <name> --no-session` | Switch without saving |
| `vibe destroy <name>` | Delete a branch and its sessions |
| `vibe sessions` | List all saved sessions |
| `vibe sessions -b <branch>` | Filter sessions by branch |
| `vibe restore <session-id>` | Restore a saved session |

### Version History

| Command | Description |
|---------|-------------|
| `vibe diff` | Diff working tree vs last commit |
| `vibe diff <hash1> <hash2>` | Diff between two commits |
| `vibe revert <hash>` | Revert to any previous commit |
| `vibe blame <file>` | Per-line authorship |

### Linking & Sync

| Command | Description |
|---------|-------------|
| `vibe link <source> [dir]` | Link to a repo (local path or URL) |
| `vibe link <url> [dir] --token <t>` | Link to a remote server with auth |
| `vibe fetch <file>` | Fetch a single file from source |
| `vibe pull` | Fetch all files from source |
| `vibe sync` | Pull latest changes from source |

### Roles & Permissions

| Command | Description |
|---------|-------------|
| `vibe roles init <name>` | Initialize roles (you become admin) |
| `vibe roles` | List all users and roles |
| `vibe grant <user> <role>` | Assign a role (`admin`, `contributor`, `reader`) |
| `vibe revoke <user>` | Remove a user's access |

**Roles:**
- **Admin** — Full control. Manage users, push, configure the repo.
- **Contributor** — Create branches, push changes.
- **Reader** — Read-only access. Can link and browse, but not modify.

### Server

| Command | Description |
|---------|-------------|
| `vibe serve` | Start the Vibe server (default port 7433) |
| `vibe serve --port 8080` | Custom port |
| `vibe serve --token mysecret` | Require auth token |
| `vibe serve --config server.toml` | Use a config file |
| `vibe ui` | Launch the web UI (default port 7434) |
| `vibe ui --server http://host:7433` | Point UI at a remote server |

## Hosting a Vibe Server

### Quick start (local)

```bash
cd my-project
vibe init
vibe roles init myname
vibe serve --port 7433
```

Others can now connect:

```bash
vibe link http://your-ip:7433 my-copy --token <their-token>
vibe pull
```

### With a config file

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

```bash
vibe serve --config my-server.toml
```

### With Docker

```dockerfile
FROM golang:1.21-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o /vibe ./cmd/vibe/

FROM alpine:latest
COPY --from=build /vibe /usr/local/bin/vibe
EXPOSE 7433
ENTRYPOINT ["vibe", "serve"]
```

```bash
docker build -t vibe-server .
docker run -p 7433:7433 -v /path/to/repo:/repo vibe-server --repo /repo
```

### With systemd

```ini
# /etc/systemd/system/vibe.service
[Unit]
Description=Vibe VCS Server
After=network.target

[Service]
ExecStart=/usr/local/bin/vibe serve --config /etc/vibe/server.toml
Restart=always
User=vibe

[Install]
WantedBy=multi-user.target
```

## Web UI

Run `vibe ui` to open the dashboard in your browser. It shows:

- Repository info (branch, HEAD, file count)
- File browser with content hashes
- Full commit history
- Branch list
- User roles
- Live event feed (real-time push notifications via WebSocket)

Pass `--server http://host:port` to point it at a remote Vibe server.

## How It Works

Vibe uses a **content-addressable object store** (similar to Git) with SHA-256 hashing. Every file, directory tree, and commit is stored as an immutable object identified by its hash.

```
.vibe/
├── HEAD                    # Current branch pointer
├── index                   # Staging area
├── roles.json              # User roles and tokens
├── objects/                # Content-addressable store
│   ├── ab/cdef1234...      # Blob, tree, or commit objects
│   └── ...
└── refs/
    ├── branches/           # Branch pointers
    │   ├── main
    │   └── feature
    └── sessions/           # Auto-saved session snapshots
        └── main/
            └── main-1234567890.json
```

**Linking** uses a hybrid sync model: when you `vibe link`, the directory structure and metadata sync immediately. File contents are fetched on-demand when you access them, then cached locally. Run `vibe pull` to download everything at once.

## Cross-Platform Builds

Vibe compiles to a single static binary. Build for any platform:

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o vibe-linux ./cmd/vibe/

# macOS
GOOS=darwin GOARCH=amd64 go build -o vibe-mac ./cmd/vibe/

# Windows
GOOS=windows GOARCH=amd64 go build -o vibe.exe ./cmd/vibe/

# ARM (Raspberry Pi, etc.)
GOOS=linux GOARCH=arm64 go build -o vibe-arm64 ./cmd/vibe/
```

## Contributing

Contributions are welcome! This project is open source.

1. Fork the repo
2. Create a branch (`vibe branch my-feature` or `git checkout -b my-feature`)
3. Make your changes
4. Run tests: `go test ./...`
5. Submit a pull request

## License

MIT License. See [LICENSE](LICENSE) for details.
