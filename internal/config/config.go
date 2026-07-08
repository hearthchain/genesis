// Package config loads the single JSON config shared by all binaries.
package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hearthchain/burning-page/internal/chain"
)

// ChainConfig is the per-chain block of the shared config: which public
// sources feed the watcher and where the burn campaign runs.
type ChainConfig struct {
	Nodes struct {
		Primary   string `json:"primary"`
		Secondary string `json:"secondary"`
	} `json:"nodes"`
	// HistoryAPI is the account-history source on chains whose node API has
	// none (the Hyperion base URL on EOS); empty where nodes serve history.
	HistoryAPI    string       `json:"historyAPI,omitempty"`
	BurnAddress   string       `json:"burnAddress"`
	Window        chain.Window `json:"window"`
	Confirmations uint64       `json:"confirmations"`
	JournalCSV    string       `json:"journalCSV"`
}

// Config is the shared runtime configuration; see config.example.json.
type Config struct {
	Chains       map[string]ChainConfig `json:"chains"`
	DataDir      string                 `json:"dataDir"`
	ListenAddr   string                 `json:"listenAddr"`
	HearthScheme string                 `json:"hearthScheme"`
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
	if len(cfg.Chains) == 0 {
		return cfg, fmt.Errorf("config: %s: at least one chains block is required", path)
	}
	for name, cc := range cfg.Chains {
		if cc.Nodes.Primary == "" || cc.Nodes.Secondary == "" || cc.BurnAddress == "" || cc.JournalCSV == "" {
			return cfg, fmt.Errorf(
				"config: %s: chain %s: nodes.primary, nodes.secondary, burnAddress and journalCSV are required",
				path, name)
		}
	}
	if len(cfg.HearthScheme) != 1 {
		return cfg, fmt.Errorf("config: %s: hearthScheme must be exactly one byte", path)
	}
	return cfg, nil
}
