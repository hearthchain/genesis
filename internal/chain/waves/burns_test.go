package waves_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/chain/waves"
)

const burnAddr = "3PHearthBurnXXXXXXXXXXXXXXXXXZgJXd1"

func burnFixtureTxs(t *testing.T) []json.RawMessage {
	t.Helper()
	var outer []json.RawMessage
	require.NoError(t, json.Unmarshal(fixture(t, "burns_window.json"), &outer))
	var txs []json.RawMessage
	require.NoError(t, json.Unmarshal(outer[0], &txs))
	return txs
}

func TestDetectBurnsFiltersWindowAssetAndRecipient(t *testing.T) {
	window := chain.Window{Start: 4000000, End: 4001000}

	burns, err := waves.DetectBurns(burnFixtureTxs(t), burnAddr, window)
	require.NoError(t, err)

	byID := map[string]chain.Burn{}
	for _, b := range burns {
		byID[b.TxID] = b
	}
	require.Len(t, burns, 4, "plain, partial, mass-transfer and not-yet-mature burns")
	assert.Equal(t, uint64(4000900), byID["Unconfirmed77"].Height, "fresh burns are detected; maturity is the watcher's call")

	assert.Equal(t, uint64(100000000), byID["BurnPlain111"].Amount)
	assert.Equal(t, "3PSenderAlice1111111111111111111111", byID["BurnPlain111"].Source)
	assert.Equal(t, uint64(4000010), byID["BurnPlain111"].Height)
	assert.Equal(t, "waves", byID["BurnPlain111"].Chain)

	assert.Equal(t, uint64(50000000), byID["BurnPartial22"].Amount)

	assert.Equal(t, uint64(30000000), byID["BurnMass3333"].Amount,
		"mass transfer sums only the entries to the burn address")
	assert.Equal(t, "3PSenderCarol333333333333333333333", byID["BurnMass3333"].Source)

	for _, absent := range []string{"NotWavesAsset", "OtherRecipien", "BeforeWindow1", "InvokeIgnored"} {
		assert.NotContains(t, byID, absent)
	}
}

func TestDetectBurnsKeepsRawJSONVerbatim(t *testing.T) {
	burns, err := waves.DetectBurns(burnFixtureTxs(t), burnAddr, chain.Window{Start: 4000000, End: 4001000})
	require.NoError(t, err)
	require.NotEmpty(t, burns)

	var round struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(burns[0].Raw, &round))
	assert.Equal(t, burns[0].TxID, round.ID, "raw node JSON travels into the artifact untouched")
}
