package ui

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

//go:embed all:static
var staticFiles embed.FS

// Serve starts the web UI on the given port.
// It proxies API calls to the Vibe server if running, or serves standalone.
func Serve(port int, serverURL, token string) error {
	// Serve embedded static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("embed fs: %w", err)
	}

	mux := http.NewServeMux()

	if serverURL != "" {
		// Proxy API requests to the Vibe server, injecting auth token
		mux.HandleFunc("/api/", proxyHandler(serverURL, token))
		// Proxy WebSocket by upgrading on both ends
		mux.HandleFunc("/ws", proxyWSHandler(serverURL, token))
	}

	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	openURL := fmt.Sprintf("http://%s", addr)
	if token != "" {
		openURL += "?token=" + token
	}

	log.Printf("Vibe UI available at %s", openURL)

	// Try to open browser
	go openBrowser(openURL)

	return http.ListenAndServe(addr, mux)
}

func proxyHandler(target, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proxyURL := target + r.URL.Path
		if r.URL.RawQuery != "" {
			proxyURL += "?" + r.URL.RawQuery
		}

		req, err := http.NewRequest(r.Method, proxyURL, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		req.Header = r.Header.Clone()
		// Inject token if the request doesn't already carry one
		if token != "" && req.Header.Get("Authorization") == "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, "server unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

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

func proxyWSHandler(target, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Pass token as query param so the WS URL includes auth
		wsTarget := target + "/ws"
		q := r.URL.RawQuery
		if token != "" {
			if q != "" {
				q += "&token=" + token
			} else {
				q = "token=" + token
			}
		}
		if q != "" {
			wsTarget += "?" + q
		}
		// Tell the browser to connect directly to the vibe server for WebSocket
		// (browsers don't follow WS redirects, so we use a JS-friendly 200 response)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ws_url":%q}`, strings.Replace(wsTarget, "http://", "ws://", 1))
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
