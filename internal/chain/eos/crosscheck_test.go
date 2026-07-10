package eos_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/chain/eos"
)

const recordedTrx = "3e2581376b6e2a3705d7225a7a6889e5afe0fc5578cc2d27b8eeb8554abd6631"

// recordedBurn rebuilds the burn the watcher would detect for the recorded
// Greymass transaction: alicewyl1235 -> eosio.null, 0.0100 A, "minepool burn".
func recordedBurn(t *testing.T) chain.Burn {
	t.Helper()
	row := action(901, 507709376, recordedTrx, "core.vaulta", "alicewyl1235", burnAccount, "0.0100 A", "minepool burn")
	burns, err := eos.DetectBurns([]json.RawMessage{row}, burnAccount, chain.Window{Start: 507000000, End: 508000000})
	require.NoError(t, err)
	require.Len(t, burns, 1)
	return burns[0]
}

func TestCrossCheckConfirmsRecordedTransaction(t *testing.T) {
	g := greymassServer(t, fixture(t, "greymass_tx.json"))

	verdict, err := eos.CrossCheck(t.Context(), g, recordedBurn(t))
	require.NoError(t, err)
	assert.Equal(t, "confirmed", verdict.Status)
	assert.Empty(t, verdict.Mismatches)
}

func TestCrossCheckPendsWhileSecondaryLags(t *testing.T) {
	var tx map[string]any
	require.NoError(t, json.Unmarshal(fixture(t, "greymass_tx.json"), &tx))
	tx["irreversible"] = false
	payload, err := json.Marshal(tx)
	require.NoError(t, err)
	g := greymassServer(t, payload)

	verdict, err := eos.CrossCheck(t.Context(), g, recordedBurn(t))
	require.NoError(t, err)
	assert.Equal(t, "pending_crosscheck", verdict.Status)
}

func TestCrossCheckFlagsDivergences(t *testing.T) {
	g := greymassServer(t, fixture(t, "greymass_tx.json"))

	tampered := recordedBurn(t)
	tampered.Amount = 999_999
	verdict, err := eos.CrossCheck(t.Context(), g, tampered)
	require.NoError(t, err)
	assert.Equal(t, "mismatch", verdict.Status)
	assert.Contains(t, verdict.Mismatches, "amount")

	moved := recordedBurn(t)
	moved.Height = 507709999
	verdict, err = eos.CrossCheck(t.Context(), g, moved)
	require.NoError(t, err)
	assert.Equal(t, "mismatch", verdict.Status)
	assert.Contains(t, verdict.Mismatches, "height")

	// A different memo in our record than on the secondary's trace.
	row := action(902, 507709376, recordedTrx, "core.vaulta", "alicewyl1235", burnAccount, "0.0100 A", "tampered memo")
	burns, err := eos.DetectBurns([]json.RawMessage{row}, burnAccount, chain.Window{Start: 507000000, End: 508000000})
	require.NoError(t, err)
	verdict, err = eos.CrossCheck(t.Context(), g, burns[0])
	require.NoError(t, err)
	assert.Equal(t, "mismatch", verdict.Status)
	assert.Contains(t, verdict.Mismatches, "transfers")
}
