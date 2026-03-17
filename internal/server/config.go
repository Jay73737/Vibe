package server

import (
	"os"

	"github.com/BurntSushi/toml"
)

// DefaultRelayURL is the built-in relay server for tunnel URL re-discovery.
// Set this to your Cloudflare Worker URL (or any hosted relay) before building.
// When non-empty, every `vibe serve --tunnel` auto-publishes to this relay,
// and every `vibe link` stores it so daemons can re-discover tunnel URLs.
// Override at runtime with VIBE_RELAY_URL env var.
var DefaultRelayURL = "https://vibe-relay.cky37373.workers.dev"

// DefaultRelayToken is the shared auth token for the default relay.
// Must match the RELAY_TOKEN configured on the relay server.
// Override at runtime with VIBE_RELAY_TOKEN env var.
var DefaultRelayToken = ""

// GetDefaultRelayURL returns the relay URL, preferring the env var override.
func GetDefaultRelayURL() string {
	if v := os.Getenv("VIBE_RELAY_URL"); v != "" {
		return v
	}
	return DefaultRelayURL
}

// GetDefaultRelayToken returns the relay token, preferring the env var override.
func GetDefaultRelayToken() string {
	if v := os.Getenv("VIBE_RELAY_TOKEN"); v != "" {
		return v
	}
	return DefaultRelayToken
}

// Config holds server configuration.
type Config struct {
	Host     string       `toml:"host"`
	Port     int          `toml:"port"`
	RepoPath string       `toml:"repo_path"`
	TLS      TLSConfig    `toml:"tls"`
	Auth     AuthConfig   `toml:"auth"`
	Tunnel   TunnelConfig `toml:"tunnel"`
	Relay    RelayConfig  `toml:"relay"`
}

// TunnelConfig controls the built-in cloudflared quick-tunnel.
type TunnelConfig struct {
	Enabled bool   `toml:"enabled"`
	Name    string `toml:"name"` // named tunnel for stable URL (optional)
}

// RelayConfig controls URL relay publishing.
type RelayConfig struct {
	URL   string `toml:"url"`   // relay server URL (e.g. http://relay.example.com)
	Token string `toml:"token"` // auth token for publishing
}

type TLSConfig struct {
	Enabled  bool   `toml:"enabled"`
	CertFile string `toml:"cert_file"`
	KeyFile  string `toml:"key_file"`
}

type AuthConfig struct {
	// Token is a shared secret for simple auth. Clients must send this
	// in the Authorization header to access the repo.
	Token string `toml:"token"`
}

func DefaultConfig() *Config {
	return &Config{
		Host:     "0.0.0.0",
		Port:     7433,
		RepoPath: ".",
	}
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
