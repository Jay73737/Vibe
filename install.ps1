#Requires -Version 5.1
<#
.SYNOPSIS
    Installs vibe - modern version control for the vibe coding era.
.DESCRIPTION
    Downloads and installs Go (if needed), builds vibe from source,
    and adds it to your PATH so you can use it from any terminal.
.PARAMETER InstallDir
    Directory to install vibe.exe into. Defaults to ~\.vibe\bin.
.PARAMETER GoVersion
    Go version to install if Go is not found. Defaults to 1.24.1.
.PARAMETER SkipGoInstall
    Skip Go installation even if not found (fail instead).
.EXAMPLE
    .\install.ps1
    irm https://raw.githubusercontent.com/vibe-vcs/vibe/main/install.ps1 | iex
#>
param(
    [string]$InstallDir = "$env:USERPROFILE\.vibe\bin",
    [string]$GoVersion = "1.24.1",
    [switch]$SkipGoInstall
)

$ErrorActionPreference = "Stop"

function Write-Step($msg) { Write-Host "=> $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "   $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "   $msg" -ForegroundColor Yellow }
function Write-Err($msg)  { Write-Host "   $msg" -ForegroundColor Red }

# ── Banner ──────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  vibe installer" -ForegroundColor Magenta
Write-Host "  modern version control for the vibe coding era" -ForegroundColor DarkGray
Write-Host ""

# ── Check architecture ──────────────────────────────────────────────
$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
$goArch = "windows-$arch"

# ── Step 1: Check / Install Go ──────────────────────────────────────
Write-Step "Checking for Go..."

$goCmd = Get-Command go -ErrorAction SilentlyContinue
if ($goCmd) {
    $goVer = & go version
    Write-Ok "Found: $goVer"
} else {
    if ($SkipGoInstall) {
        Write-Err "Go is not installed and -SkipGoInstall was specified."
        Write-Err "Install Go from https://go.dev/dl/ and try again."
        exit 1
    }

    Write-Warn "Go not found. Installing Go $GoVersion..."

    $goMsi = "go$GoVersion.$goArch.msi"
    $goUrl = "https://go.dev/dl/$goMsi"
    $tmpMsi = Join-Path $env:TEMP $goMsi

    Write-Step "Downloading $goUrl..."
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -Uri $goUrl -OutFile $tmpMsi -UseBasicParsing

    Write-Step "Installing Go (this may require admin privileges)..."
    $msiArgs = "/i `"$tmpMsi`" /quiet /norestart"
    $proc = Start-Process msiexec.exe -ArgumentList $msiArgs -Wait -PassThru
    if ($proc.ExitCode -ne 0) {
        Write-Err "Go installation failed (exit code $($proc.ExitCode))."
        Write-Err "Try installing Go manually from https://go.dev/dl/"
        exit 1
    }

    Remove-Item $tmpMsi -ErrorAction SilentlyContinue

    # Refresh PATH so we can find go.exe
    $machinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $env:Path = "$machinePath;$userPath"

    $goCmd = Get-Command go -ErrorAction SilentlyContinue
    if (-not $goCmd) {
        # Try the default install location
        $defaultGo = "C:\Program Files\Go\bin"
        if (Test-Path "$defaultGo\go.exe") {
            $env:Path = "$env:Path;$defaultGo"
        } else {
            Write-Err "Go was installed but cannot be found on PATH."
            Write-Err "Restart your terminal and run this script again."
            exit 1
        }
    }

    $goVer = & go version
    Write-Ok "Installed: $goVer"
}

# ── Step 1b: Check / Install cloudflared ──────────────────────────────
Write-Step "Checking for cloudflared (tunnel support)..."

$cfCmd = Get-Command cloudflared -ErrorAction SilentlyContinue
if ($cfCmd) {
    $cfVer = & cloudflared version 2>&1
    Write-Ok "Found: $cfVer"
} else {
    Write-Warn "cloudflared not found. Installing via winget..."

    $wingetCmd = Get-Command winget -ErrorAction SilentlyContinue
    if ($wingetCmd) {
        & winget install --id Cloudflare.cloudflared --accept-package-agreements --accept-source-agreements --silent 2>&1 | Out-Null
        if ($LASTEXITCODE -eq 0) {
            # Refresh PATH
            $machinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
            $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
            $env:Path = "$machinePath;$userPath"
            Write-Ok "Installed cloudflared."
        } else {
            Write-Warn "winget install failed. You can install cloudflared manually later:"
            Write-Warn "  winget install Cloudflare.cloudflared"
            Write-Warn "  https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/"
        }
    } else {
        Write-Warn "winget not available. You can install cloudflared manually later:"
        Write-Warn "  https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/"
    }
}

# ── Step 2: Get vibe source ─────────────────────────────────────────
Write-Step "Preparing vibe source..."

# Check if we're already in the vibe source tree
$sourceDir = $null
if (Test-Path ".\go.mod") {
    $modContent = Get-Content ".\go.mod" -First 1
    if ($modContent -match "github.com/vibe-vcs/vibe") {
        $sourceDir = (Get-Location).Path
        Write-Ok "Using local source: $sourceDir"
    }
}

if (-not $sourceDir) {
    # Clone from GitHub
    $sourceDir = Join-Path $env:TEMP "vibe-build-$(Get-Random)"
    Write-Step "Cloning vibe repository..."

    $gitCmd = Get-Command git -ErrorAction SilentlyContinue
    if ($gitCmd) {
        & git clone --depth 1 https://github.com/Jay73737/Vibe.git $sourceDir
        if ($LASTEXITCODE -ne 0) {
            Write-Err "Failed to clone repository."
            exit 1
        }
    } else {
        Write-Warn "Git not found. Downloading source archive..."
        $zipUrl = "https://github.com/vibe-vcs/vibe/archive/refs/heads/main.zip"
        $zipPath = Join-Path $env:TEMP "vibe-src.zip"
        Invoke-WebRequest -Uri $zipUrl -OutFile $zipPath -UseBasicParsing
        Expand-Archive -Path $zipPath -DestinationPath $env:TEMP -Force
        $sourceDir = Join-Path $env:TEMP "vibe-main"
        Remove-Item $zipPath -ErrorAction SilentlyContinue
    }
}

# ── Step 3: Build vibe ──────────────────────────────────────────────
Write-Step "Building vibe..."

Push-Location $sourceDir
try {
    & go build -o vibe.exe ./cmd/vibe
    if ($LASTEXITCODE -ne 0) {
        Write-Err "Build failed."
        exit 1
    }
    Write-Ok "Build successful."
} finally {
    Pop-Location
}

# ── Step 4: Install binary ──────────────────────────────────────────
Write-Step "Installing to $InstallDir..."

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$srcExe = Join-Path $sourceDir "vibe.exe"
$dstExe = Join-Path $InstallDir "vibe.exe"

Copy-Item $srcExe $dstExe -Force
Write-Ok "Installed: $dstExe"

# ── Step 5: Update PATH ─────────────────────────────────────────────
Write-Step "Checking PATH..."

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -split ";" | Where-Object { $_ -eq $InstallDir }) {
    Write-Ok "Already on PATH."
} else {
    $newPath = if ($userPath) { "$userPath;$InstallDir" } else { $InstallDir }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    $env:Path = "$env:Path;$InstallDir"
    Write-Ok "Added $InstallDir to user PATH."
}

# ── Step 6: Verify ──────────────────────────────────────────────────
Write-Step "Verifying installation..."

$vibeVer = & $dstExe version 2>&1
Write-Ok "$vibeVer"

# ── Cleanup temp source if we cloned ────────────────────────────────
if ($sourceDir -like "$env:TEMP*") {
    Remove-Item $sourceDir -Recurse -Force -ErrorAction SilentlyContinue
}

# ── Step 7: Install daemon service ──────────────────────────────────
Write-Step "Installing vibe daemon as a startup service..."

try {
    & $dstExe service install 2>&1 | Out-Null
    if ($LASTEXITCODE -eq 0) {
        Write-Ok "Daemon service installed (runs at logon)."
        & $dstExe service start 2>&1 | Out-Null
        if ($LASTEXITCODE -eq 0) {
            Write-Ok "Daemon service started."
        } else {
            Write-Warn "Could not start daemon now. It will start on next logon."
        }
    } else {
        Write-Warn "Could not install daemon service automatically."
        Write-Warn "You can install it manually later: vibe service install"
    }
} catch {
    Write-Warn "Could not install daemon service: $_"
    Write-Warn "You can install it manually later: vibe service install"
}

# ── Done ─────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  vibe is installed!" -ForegroundColor Green
Write-Host ""
Write-Host "  Open a NEW terminal, then run:" -ForegroundColor White
Write-Host "    vibe help" -ForegroundColor Yellow
Write-Host ""
Write-Host "  Quick start:" -ForegroundColor White
Write-Host "    mkdir myproject && cd myproject" -ForegroundColor DarkGray
Write-Host "    vibe init" -ForegroundColor DarkGray
Write-Host "    vibe save 'first commit'" -ForegroundColor DarkGray
Write-Host ""
Write-Host "  Share with anyone:" -ForegroundColor White
Write-Host "    vibe serve --tunnel" -ForegroundColor DarkGray
Write-Host "    vibe invite <name>" -ForegroundColor DarkGray
Write-Host ""
Write-Host "  Background sync daemon:" -ForegroundColor White
Write-Host "    vibe service status     Check daemon status" -ForegroundColor DarkGray
Write-Host "    vibe service install    Install as startup service" -ForegroundColor DarkGray
Write-Host "    vibe daemon             Run in foreground (debug)" -ForegroundColor DarkGray
Write-Host ""
