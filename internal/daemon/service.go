package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ServiceInstall installs the vibe daemon as a startup service.
func ServiceInstall() error {
	vibeBin, err := findVibeBinary()
	if err != nil {
		return fmt.Errorf("cannot find vibe binary: %w", err)
	}

	switch runtime.GOOS {
	case "windows":
		return installWindows(vibeBin)
	case "linux":
		return installLinux(vibeBin)
	case "darwin":
		return installMacOS(vibeBin)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// ServiceUninstall removes the vibe daemon startup service.
func ServiceUninstall() error {
	switch runtime.GOOS {
	case "windows":
		return uninstallWindows()
	case "linux":
		return uninstallLinux()
	case "darwin":
		return uninstallMacOS()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// ServiceStart starts the daemon service.
func ServiceStart() error {
	switch runtime.GOOS {
	case "windows":
		return startWindows()
	case "linux":
		return startLinux()
	case "darwin":
		return startMacOS()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// ServiceStop stops the daemon service.
func ServiceStop() error {
	switch runtime.GOOS {
	case "windows":
		return stopWindows()
	case "linux":
		return stopLinux()
	case "darwin":
		return stopMacOS()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// ServiceStatus checks whether the daemon service is running.
func ServiceStatus() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return statusWindows()
	case "linux":
		return statusLinux()
	case "darwin":
		return statusMacOS()
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// ── Windows: Task Scheduler ─────────────────────────────────────────

const winTaskName = "VibeDaemon"

func installWindows(vibeBin string) error {
	// Remove existing task if any
	exec.Command("schtasks", "/delete", "/tn", winTaskName, "/f").Run()

	// Create a task that runs at logon
	cmd := exec.Command("schtasks", "/create",
		"/tn", winTaskName,
		"/tr", fmt.Sprintf(`"%s" daemon`, vibeBin),
		"/sc", "onlogon",
		"/rl", "limited",
		"/f",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks create: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func uninstallWindows() error {
	cmd := exec.Command("schtasks", "/delete", "/tn", winTaskName, "/f")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks delete: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func startWindows() error {
	cmd := exec.Command("schtasks", "/run", "/tn", winTaskName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks run: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func stopWindows() error {
	// Kill the daemon process
	cmd := exec.Command("taskkill", "/f", "/im", "vibe.exe", "/fi", "WINDOWTITLE eq vibe*")
	cmd.Run() // best effort

	cmd = exec.Command("schtasks", "/end", "/tn", winTaskName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Also try killing by process name as fallback
		exec.Command("taskkill", "/f", "/im", "vibe.exe").Run()
		return fmt.Errorf("schtasks end: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func statusWindows() (string, error) {
	cmd := exec.Command("schtasks", "/query", "/tn", winTaskName, "/fo", "list")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "not installed", nil
	}
	output := string(out)
	if strings.Contains(output, "Running") {
		return "running", nil
	}
	if strings.Contains(output, "Ready") {
		return "installed (not running)", nil
	}
	return "installed", nil
}

// ── Linux: systemd user service ─────────────────────────────────────

const systemdServiceName = "vibe-daemon"

func systemdUserDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user")
}

func systemdServicePath() string {
	return filepath.Join(systemdUserDir(), systemdServiceName+".service")
}

func systemdServiceContent(vibeBin string) string {
	return fmt.Sprintf(`[Unit]
Description=Vibe VCS Daemon - automatic sync service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s daemon
Restart=always
RestartSec=5
Environment=HOME=%s

[Install]
WantedBy=default.target
`, vibeBin, os.Getenv("HOME"))
}

func installLinux(vibeBin string) error {
	dir := systemdUserDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create systemd dir: %w", err)
	}

	content := systemdServiceContent(vibeBin)
	if err := os.WriteFile(systemdServicePath(), []byte(content), 0644); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}

	// Reload and enable
	if out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("daemon-reload: %s (%w)", string(out), err)
	}
	if out, err := exec.Command("systemctl", "--user", "enable", systemdServiceName).CombinedOutput(); err != nil {
		return fmt.Errorf("enable: %s (%w)", string(out), err)
	}

	// Enable lingering so the service runs even when not logged in
	user := os.Getenv("USER")
	if user != "" {
		exec.Command("loginctl", "enable-linger", user).Run() // best effort
	}

	return nil
}

func uninstallLinux() error {
	exec.Command("systemctl", "--user", "stop", systemdServiceName).Run()
	exec.Command("systemctl", "--user", "disable", systemdServiceName).Run()
	os.Remove(systemdServicePath())
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}

func startLinux() error {
	out, err := exec.Command("systemctl", "--user", "start", systemdServiceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("start: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func stopLinux() error {
	out, err := exec.Command("systemctl", "--user", "stop", systemdServiceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("stop: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func statusLinux() (string, error) {
	cmd := exec.Command("systemctl", "--user", "is-active", systemdServiceName)
	out, _ := cmd.CombinedOutput()
	state := strings.TrimSpace(string(out))
	switch state {
	case "active":
		return "running", nil
	case "inactive":
		return "installed (not running)", nil
	case "failed":
		return "failed", nil
	default:
		// Check if the service file exists
		if _, err := os.Stat(systemdServicePath()); err != nil {
			return "not installed", nil
		}
		return state, nil
	}
}

// ── macOS: launchd user agent ───────────────────────────────────────

const launchdLabel = "com.vibe-vcs.daemon"

func launchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func launchdPlistContent(vibeBin string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>daemon</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/vibe-daemon.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/vibe-daemon.log</string>
</dict>
</plist>
`, launchdLabel, vibeBin)
}

func installMacOS(vibeBin string) error {
	dir := filepath.Dir(launchdPlistPath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	content := launchdPlistContent(vibeBin)
	if err := os.WriteFile(launchdPlistPath(), []byte(content), 0644); err != nil {
		return err
	}
	return nil
}

func uninstallMacOS() error {
	exec.Command("launchctl", "unload", launchdPlistPath()).Run()
	os.Remove(launchdPlistPath())
	return nil
}

func startMacOS() error {
	out, err := exec.Command("launchctl", "load", launchdPlistPath()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl load: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func stopMacOS() error {
	out, err := exec.Command("launchctl", "unload", launchdPlistPath()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl unload: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func statusMacOS() (string, error) {
	cmd := exec.Command("launchctl", "list", launchdLabel)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if _, statErr := os.Stat(launchdPlistPath()); statErr != nil {
			return "not installed", nil
		}
		return "installed (not running)", nil
	}
	if strings.Contains(string(out), launchdLabel) {
		return "running", nil
	}
	return "installed", nil
}

// ── Helpers ─────────────────────────────────────────────────────────

func findVibeBinary() (string, error) {
	// Try the running binary first
	exe, err := os.Executable()
	if err == nil {
		abs, err := filepath.Abs(exe)
		if err == nil {
			return abs, nil
		}
		return exe, nil
	}

	// Fallback: search PATH
	path, err := exec.LookPath("vibe")
	if err != nil {
		return "", fmt.Errorf("vibe binary not found in PATH")
	}
	return filepath.Abs(path)
}
