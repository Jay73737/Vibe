#!/usr/bin/env bash
set -euo pipefail

# vibe installer - modern version control for the vibe coding era
# Usage:
#   ./install.sh
#   curl -fsSL https://raw.githubusercontent.com/Jay73737/Vibe/main/install.sh | bash

GO_VERSION="${GO_VERSION:-1.24.1}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.vibe/bin}"

# ── Helpers ──────────────────────────────────────────────────────────
step()  { printf '\033[36m=> %s\033[0m\n' "$*"; }
ok()    { printf '\033[32m   %s\033[0m\n' "$*"; }
warn()  { printf '\033[33m   %s\033[0m\n' "$*"; }
err()   { printf '\033[31m   %s\033[0m\n' "$*"; }
die()   { err "$@"; exit 1; }

# ── Banner ───────────────────────────────────────────────────────────
echo ""
printf '\033[35m  vibe installer\033[0m\n'
printf '\033[90m  modern version control for the vibe coding era\033[0m\n'
echo ""

# ── Detect OS and architecture ───────────────────────────────────────
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
    linux*)  GOOS="linux" ;;
    darwin*) GOOS="darwin" ;;
    *)       die "Unsupported OS: $OS (use install.ps1 for Windows)" ;;
esac

case "$ARCH" in
    x86_64|amd64)  GOARCH="amd64" ;;
    arm64|aarch64) GOARCH="arm64" ;;
    armv6l)        GOARCH="armv6l" ;;
    i386|i686)     GOARCH="386" ;;
    *)             die "Unsupported architecture: $ARCH" ;;
esac

# ── Step 1: Check / Install Go ──────────────────────────────────────
step "Checking for Go..."

if command -v go &>/dev/null; then
    ok "Found: $(go version)"
else
    warn "Go not found. Installing Go $GO_VERSION..."

    GO_TAR="go${GO_VERSION}.${GOOS}-${GOARCH}.tar.gz"
    GO_URL="https://go.dev/dl/$GO_TAR"
    TMP_DIR="$(mktemp -d)"

    step "Downloading $GO_URL..."
    if command -v curl &>/dev/null; then
        curl -fsSL "$GO_URL" -o "$TMP_DIR/$GO_TAR"
    elif command -v wget &>/dev/null; then
        wget -q "$GO_URL" -O "$TMP_DIR/$GO_TAR"
    else
        die "Neither curl nor wget found. Install one and try again."
    fi

    step "Installing Go to /usr/local/go..."
    if [ -w "/usr/local" ]; then
        rm -rf /usr/local/go
        tar -C /usr/local -xzf "$TMP_DIR/$GO_TAR"
    else
        warn "Need sudo to install Go to /usr/local..."
        sudo rm -rf /usr/local/go
        sudo tar -C /usr/local -xzf "$TMP_DIR/$GO_TAR"
    fi

    rm -rf "$TMP_DIR"
    export PATH="/usr/local/go/bin:$PATH"

    if ! command -v go &>/dev/null; then
        die "Go was installed but cannot be found. Restart your shell and try again."
    fi

    ok "Installed: $(go version)"
fi

# ── Step 1b: Check / Install cloudflared ──────────────────────────────
step "Checking for cloudflared (tunnel support)..."

if command -v cloudflared &>/dev/null; then
    ok "Found: $(cloudflared version 2>&1 | head -1)"
else
    warn "cloudflared not found. Installing..."

    CF_INSTALLED=false

    # Try package managers in order of preference
    if command -v brew &>/dev/null; then
        step "Installing via Homebrew..."
        brew install cloudflared && CF_INSTALLED=true
    elif command -v apt-get &>/dev/null; then
        step "Installing via apt..."
        # Add Cloudflare GPG key and repo
        if [ -w "/usr/share/keyrings" ] || [ -w "/etc/apt" ]; then
            curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg | tee /usr/share/keyrings/cloudflare-main.gpg >/dev/null
            echo "deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared $(lsb_release -cs 2>/dev/null || echo stable) main" | tee /etc/apt/sources.list.d/cloudflared.list >/dev/null
            apt-get update -qq && apt-get install -y -qq cloudflared && CF_INSTALLED=true
        else
            warn "Need sudo to install cloudflared via apt..."
            curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg | sudo tee /usr/share/keyrings/cloudflare-main.gpg >/dev/null
            echo "deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared $(lsb_release -cs 2>/dev/null || echo stable) main" | sudo tee /etc/apt/sources.list.d/cloudflared.list >/dev/null
            sudo apt-get update -qq && sudo apt-get install -y -qq cloudflared && CF_INSTALLED=true
        fi
    fi

    # Fallback: direct binary download
    if [ "$CF_INSTALLED" = false ]; then
        step "Downloading cloudflared binary..."
        CF_URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-${GOOS}-${GOARCH}"
        CF_BIN="$INSTALL_DIR/cloudflared"
        mkdir -p "$INSTALL_DIR"
        if curl -fsSL "$CF_URL" -o "$CF_BIN" 2>/dev/null; then
            chmod +x "$CF_BIN"
            CF_INSTALLED=true
            ok "Installed: $CF_BIN"
        else
            warn "Could not download cloudflared. You can install it manually later:"
            warn "  https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/"
        fi
    fi

    if [ "$CF_INSTALLED" = true ]; then
        ok "cloudflared installed."
    fi
fi

# ── Step 2: Get vibe source ─────────────────────────────────────────
step "Preparing vibe source..."

SOURCE_DIR=""
CLEANUP_SOURCE=false

if [ -f "./go.mod" ] && head -1 ./go.mod | grep -q "vibe-vcs/vibe"; then
    SOURCE_DIR="$(pwd)"
    ok "Using local source: $SOURCE_DIR"
else
    SOURCE_DIR="$(mktemp -d)"
    CLEANUP_SOURCE=true

    if command -v git &>/dev/null; then
        step "Cloning vibe repository..."
        git clone --depth 1 https://github.com/Jay73737/Vibe.git "$SOURCE_DIR"
    else
        step "Downloading source archive..."
        TMP_ZIP="$(mktemp)"
        if command -v curl &>/dev/null; then
            curl -fsSL "https://github.com/Jay73737/Vibe/archive/refs/heads/main.tar.gz" -o "$TMP_ZIP"
        else
            wget -q "https://github.com/Jay73737/Vibe/archive/refs/heads/main.tar.gz" -O "$TMP_ZIP"
        fi
        tar -xzf "$TMP_ZIP" -C "$SOURCE_DIR" --strip-components=1
        rm -f "$TMP_ZIP"
    fi
fi

# ── Step 3: Build vibe ──────────────────────────────────────────────
step "Building vibe..."

(cd "$SOURCE_DIR" && go build -o vibe ./cmd/vibe)
ok "Build successful."

# ── Step 4: Install binary ──────────────────────────────────────────
step "Installing to $INSTALL_DIR..."

mkdir -p "$INSTALL_DIR"

# Stop and uninstall the daemon service before replacing the binary,
# so systemd can't auto-restart it mid-install.
if command -v "$INSTALL_DIR/vibe" &>/dev/null; then
    "$INSTALL_DIR/vibe" service stop      2>/dev/null || true
    "$INSTALL_DIR/vibe" service uninstall 2>/dev/null || true
fi
# Force-kill any remaining vibe process
pkill -x vibe 2>/dev/null || true
sleep 1

# Use mv (rename syscall) to atomically replace the binary.
# mv never opens the destination file so it cannot hit ETXTBSY,
# even if a stale process still has the old inode mapped.
cp "$SOURCE_DIR/vibe" "$INSTALL_DIR/vibe.new"
chmod +x "$INSTALL_DIR/vibe.new"
mv -f "$INSTALL_DIR/vibe.new" "$INSTALL_DIR/vibe"
ok "Installed: $INSTALL_DIR/vibe"

# ── Step 5: Update PATH ─────────────────────────────────────────────
step "Checking PATH..."

add_to_path() {
    local line="export PATH=\"$INSTALL_DIR:\$PATH\""

    for rc in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile"; do
        if [ -f "$rc" ]; then
            if ! grep -qF "$INSTALL_DIR" "$rc"; then
                echo "" >> "$rc"
                echo "# vibe version control" >> "$rc"
                echo "$line" >> "$rc"
                ok "Added to $rc"
            fi
        fi
    done
}

if echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    ok "Already on PATH."
else
    add_to_path
    export PATH="$INSTALL_DIR:$PATH"
    warn "PATH updated. Restart your shell or run: source ~/.bashrc"
fi

# ── Step 6: Verify ──────────────────────────────────────────────────
step "Verifying installation..."
ok "$("$INSTALL_DIR/vibe" version 2>&1)"

# ── Cleanup ──────────────────────────────────────────────────────────
if [ "$CLEANUP_SOURCE" = true ]; then
    rm -rf "$SOURCE_DIR"
fi

# ── Step 7: Install daemon service ───────────────────────────────────
step "Installing vibe daemon as a startup service..."

if "$INSTALL_DIR/vibe" service install 2>/dev/null; then
    ok "Daemon service installed."
    if "$INSTALL_DIR/vibe" service start 2>/dev/null; then
        ok "Daemon service started."
    else
        warn "Could not start daemon now. It will start on next login."
    fi
else
    warn "Could not install daemon service automatically."
    warn "You can install it manually later: vibe service install"
fi

# ── Done ─────────────────────────────────────────────────────────────
echo ""
printf '\033[32m  vibe is installed!\033[0m\n'
echo ""
printf '\033[37m  Open a NEW terminal, then run:\033[0m\n'
printf '\033[33m    vibe help\033[0m\n'
echo ""
printf '\033[37m  Quick start:\033[0m\n'
printf '\033[90m    mkdir myproject && cd myproject\033[0m\n'
printf '\033[90m    vibe init\033[0m\n'
printf '\033[90m    vibe save "first commit"\033[0m\n'
echo ""
printf '\033[37m  Share with anyone:\033[0m\n'
printf '\033[90m    vibe serve --tunnel\033[0m\n'
printf '\033[90m    vibe invite <name>\033[0m\n'
echo ""
printf '\033[37m  Background sync daemon:\033[0m\n'
printf '\033[90m    vibe service status     Check daemon status\033[0m\n'
printf '\033[90m    vibe service install    Install as startup service\033[0m\n'
printf '\033[90m    vibe daemon             Run in foreground (debug)\033[0m\n'
echo ""
