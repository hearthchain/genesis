package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/config"
)

func TestLoadExampleConfig(t *testing.T) {
	cfg, err := config.Load("../../config.example.json")
	require.NoError(t, err)

	assert.Equal(t, "https://nodes.wavesnodes.com", cfg.Nodes.Primary)
	assert.Equal(t, "https://nodes.wx.network", cfg.Nodes.Secondary)
	assert.Equal(t, "3PHearthBurnXXXXXXXXXXXXXXXXXZgJXd1", cfg.BurnAddress)
	assert.Equal(t, uint64(100), cfg.Confirmations)
	assert.Equal(t, "data", cfg.DataDir)
	assert.Equal(t, "data/journal/waves.csv", cfg.JournalCSV)
	assert.Equal(t, ":8080", cfg.ListenAddr)
	assert.Equal(t, byte('H'), cfg.HearthSchemeByte())
	assert.Equal(t, []string{"https://genesis.hearth.tech"}, cfg.AllowedOrigins)
}

func TestLoadRejectsMissingFileAndBadScheme(t *testing.T) {
	_, err := config.Load("does-not-exist.json")
	assert.Error(t, err)
}
