package server

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Tunnel manages a cloudflared quick-tunnel subprocess.
type Tunnel struct {
	URL     string // public URL once established
	cmd     *exec.Cmd
	mu      sync.Mutex
	done    chan struct{}
	vibeDir string // .vibe dir path, used to persist the tunnel URL
}

// urlFile returns the path where the tunnel URL is persisted so other
// commands (like "vibe invite") can read it.
func tunnelURLFile(vibeDir string) string {
	return filepath.Join(vibeDir, "tunnel_url")
}

// ReadTunnelURL reads a previously written tunnel URL from disk.
// Returns empty string if no tunnel is active.
func ReadTunnelURL(vibeDir string) string {
	data, err := os.ReadFile(tunnelURLFile(vibeDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// StartTunnel launches `cloudflared tunnel --url http://localhost:<port>` and
// blocks until the public URL is captured or the timeout expires.
func StartTunnel(port int, vibeDir string) (*Tunnel, error) {
	// Check that cloudflared is installed
	path, err := exec.LookPath("cloudflared")
	if err != nil {
		return nil, fmt.Errorf("cloudflared not found in PATH — install it from https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/")
	}

	localURL := fmt.Sprintf("http://localhost:%d", port)
	cmd := exec.Command(path, "tunnel", "--url", localURL)

	// cloudflared prints the public URL to stderr
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to capture cloudflared output: %w", err)
	}

	t := &Tunnel{
		cmd:     cmd,
		done:    make(chan struct{}),
		vibeDir: vibeDir,
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start cloudflared: %w", err)
	}

	// Parse the public URL from stderr output
	urlCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		re := regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("[cloudflared] %s", line)
			if match := re.FindString(line); match != "" {
				select {
				case urlCh <- match:
				default:
				}
			}
		}
	}()

	// Wait for the URL or timeout
	select {
	case url := <-urlCh:
		t.mu.Lock()
		t.URL = url
		t.mu.Unlock()
		// Persist the URL so "vibe invite" can read it
		os.WriteFile(tunnelURLFile(vibeDir), []byte(url+"\n"), 0644)
		log.Printf("Tunnel established: %s -> %s", url, localURL)
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		return nil, fmt.Errorf("timed out waiting for cloudflared to establish tunnel (30s)")
	}

	// Monitor the process in the background
	go func() {
		cmd.Wait()
		// Clean up the URL file when the tunnel dies
		os.Remove(tunnelURLFile(vibeDir))
		close(t.done)
	}()

	return t, nil
}

// Stop gracefully shuts down the tunnel.
func (t *Tunnel) Stop() {
	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
		<-t.done
	}
	os.Remove(tunnelURLFile(t.vibeDir))
}
