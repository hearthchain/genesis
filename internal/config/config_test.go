package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/config"
)

func TestLoadExampleConfig(t *testing.T) {
	cfg, err := config.Load("../../config.example.json")
	require.NoError(t, err)

	waves, ok := cfg.Chains["waves"]
	require.True(t, ok, "the example config carries a waves chain block")
	assert.Equal(t, "https://nodes.wavesnodes.com", waves.Nodes.Primary)
	assert.Equal(t, "https://nodes.wx.network", waves.Nodes.Secondary)
	assert.Equal(t, "3PHearthBurnXXXXXXXXXXXXXXXXXZgJXd1", waves.BurnAddress)
	assert.Equal(t, uint64(100), waves.Confirmations)
	assert.Equal(t, "data/journal/waves.csv", waves.JournalCSV)
	assert.Equal(t, "data", cfg.DataDir)
	assert.Equal(t, ":8080", cfg.ListenAddr)
	assert.Equal(t, byte('H'), cfg.HearthSchemeByte())
	assert.Equal(t, []string{"https://hearth.tech"}, cfg.AllowedOrigins)
}

func TestLoadRejectsMissingFileAndBadScheme(t *testing.T) {
	_, err := config.Load("does-not-exist.json")
	assert.Error(t, err)
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestLoadRejectsConfigWithoutChains(t *testing.T) {
	path := writeConfig(t, `{"dataDir":"data","listenAddr":":8080","hearthScheme":"H"}`)

	_, err := config.Load(path)
	assert.ErrorContains(t, err, "chains")
}

func TestLoadRejectsChainMissingRequiredFields(t *testing.T) {
	path := writeConfig(t, `{
		"chains": {"waves": {"nodes": {"primary": "https://a"}, "burnAddress": "", "journalCSV": ""}},
		"dataDir": "data", "hearthScheme": "H"
	}`)

	_, err := config.Load(path)
	assert.ErrorContains(t, err, "waves")
}
