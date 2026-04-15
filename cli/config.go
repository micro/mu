// Package cli provides the `mu` command-line interface. It is a thin
// client that talks to any Mu instance's MCP endpoint over HTTP. It has
// no dependencies on the rest of the Mu codebase and no embedded data —
// every command is dispatched via JSON-RPC against /mcp.
//
// This file handles configuration: loading/saving the on-disk config,
// merging environment variables, and applying CLI flag overrides.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultURL is used when nothing else is configured.
const DefaultURL = "https://mu.xyz"

// Config is the on-disk configuration loaded from
// $XDG_CONFIG_HOME/mu/config.json (or ~/.config/mu/config.json).
type Config struct {
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}

// configDir returns the directory where the config file lives,
// creating it if necessary.
func configDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	dir := filepath.Join(base, "mu")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// configPath returns the full path to the config file.
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// LoadConfig reads the config from disk. Returns a zero Config if the
// file doesn't exist.
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &c, nil
}

// SaveConfig writes the config to disk with restrictive permissions so
// the token isn't world-readable.
func SaveConfig(c *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// ResolvedConfig is the effective configuration after merging file,
// environment and flag overrides.
type ResolvedConfig struct {
	URL     string
	Token   string
	Pretty  bool // force pretty output regardless of tty
	Raw     bool // force raw output (no pretty)
	Table   bool // prefer table layout for list results
	Verbose bool
}

// Resolve merges the sources in priority order: flag overrides > env >
// file > defaults.
func (r *ResolvedConfig) Apply(file *Config) {
	if r.URL == "" {
		r.URL = os.Getenv("MU_URL")
	}
	if r.URL == "" && file != nil {
		r.URL = file.URL
	}
	if r.URL == "" {
		r.URL = DefaultURL
	}

	if r.Token == "" {
		r.Token = os.Getenv("MU_TOKEN")
	}
	if r.Token == "" && file != nil {
		r.Token = file.Token
	}
}

// Validate returns an error when the configuration is missing
// something required for a call.
func (r *ResolvedConfig) Validate() error {
	if r.URL == "" {
		return errors.New("no Mu URL configured (set MU_URL, run `mu login`, or pass --url)")
	}
	return nil
}
