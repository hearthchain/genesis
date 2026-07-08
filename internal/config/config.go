// Package config loads the single JSON config shared by all binaries.
package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hearthchain/burning-page/internal/chain"
)

// Config is the shared runtime configuration; see config.example.json.
type Config struct {
	Nodes struct {
		Primary   string `json:"primary"`
		Secondary string `json:"secondary"`
	} `json:"nodes"`
	BurnAddress   string       `json:"burnAddress"`
	Window        chain.Window `json:"window"`
	Confirmations uint64       `json:"confirmations"`
	DataDir       string       `json:"dataDir"`
	JournalCSV    string       `json:"journalCSV"`
	ListenAddr    string       `json:"listenAddr"`
	HearthScheme  string       `json:"hearthScheme"`
	// AllowedOrigins lists the web origins the API answers CORS for; empty
	// means no CORS headers (same-origin deployments need none).
	AllowedOrigins []string `json:"allowedOrigins"`
}

// HearthSchemeByte returns the Hearth address scheme byte.
func (c Config) HearthSchemeByte() byte {
	return c.HearthScheme[0]
}

// Load reads and validates a config file.
func Load(path string) (Config, error) {
	var cfg Config
	raw, err := os.ReadFile(path) //nolint:gosec // the config path is an operator-supplied flag
	if err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}
	if uErr := json.Unmarshal(raw, &cfg); uErr != nil {
		return cfg, fmt.Errorf("config: %s: %w", path, uErr)
	}
	if cfg.Nodes.Primary == "" || cfg.Nodes.Secondary == "" || cfg.BurnAddress == "" {
		return cfg, fmt.Errorf("config: %s: nodes.primary, nodes.secondary and burnAddress are required", path)
	}
	if len(cfg.HearthScheme) != 1 {
		return cfg, fmt.Errorf("config: %s: hearthScheme must be exactly one byte", path)
	}
	return cfg, nil
}
