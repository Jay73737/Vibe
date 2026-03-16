package server

import (
	"os"

	"github.com/BurntSushi/toml"
)

// Config holds server configuration.
type Config struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	RepoPath string `toml:"repo_path"`
	TLS      TLSConfig `toml:"tls"`
	Auth     AuthConfig `toml:"auth"`
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
