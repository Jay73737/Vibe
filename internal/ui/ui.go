package ui

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os/exec"
	"runtime"
)

//go:embed all:static
var staticFiles embed.FS

// Serve starts the web UI on the given port.
// It proxies API calls to the Vibe server if running, or serves standalone.
func Serve(port int, serverURL string) error {
	// Serve embedded static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("embed fs: %w", err)
	}

	mux := http.NewServeMux()

	if serverURL != "" {
		// Proxy API requests to the Vibe server
		mux.HandleFunc("/api/", proxyHandler(serverURL))
		mux.HandleFunc("/ws", proxyWSHandler(serverURL))
	}

	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	url := fmt.Sprintf("http://%s", addr)

	log.Printf("Vibe UI available at %s", url)

	// Try to open browser
	go openBrowser(url)

	return http.ListenAndServe(addr, mux)
}

func proxyHandler(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Simple reverse proxy
		proxyURL := target + r.URL.Path
		if r.URL.RawQuery != "" {
			proxyURL += "?" + r.URL.RawQuery
		}

		req, err := http.NewRequest(r.Method, proxyURL, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		// Forward auth headers
		req.Header = r.Header.Clone()

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, "server unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Copy response
		for k, v := range resp.Header {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
		w.WriteHeader(resp.StatusCode)
		buf := make([]byte, 32*1024)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
	}
}

func proxyWSHandler(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For WebSocket, redirect client to connect directly to the server
		http.Redirect(w, r, target+"/ws?"+r.URL.RawQuery, http.StatusTemporaryRedirect)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}
