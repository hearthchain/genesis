package eos_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/chain/eos"
)

const burnAccount = "eosio.null"

func TestDetectBurnsSumsPerTransactionInsideWindow(t *testing.T) {
	window := chain.Window{Start: 508400000, End: 508500000}
	rows := []json.RawMessage{
		action(601, 508400100, "burn01", "core.vaulta", me, burnAccount, "2.0000 A", ""),
		// One transaction burning both tokens: summed into one burn.
		action(602, 508400200, "burn02", "core.vaulta", me, burnAccount, "1.0000 A", "combined"),
		action(603, 508400200, "burn02", "eosio.token", me, burnAccount, "0.5000 EOS", "combined"),
		// Outside the window on both sides.
		action(604, 508399999, "early1", "core.vaulta", me, burnAccount, "9.0000 A", ""),
		action(605, 508500001, "late01", "core.vaulta", me, burnAccount, "9.0000 A", ""),
		// A row not addressed to the burn account is ignored, not a burn.
		action(606, 508400300, "aside1", "core.vaulta", me, "someoneelse1", "3.0000 A", ""),
	}

	burns, err := eos.DetectBurns(rows, burnAccount, window)
	require.NoError(t, err)
	require.Len(t, burns, 2)

	assert.Equal(t, "burn01", burns[0].TxID)
	assert.Equal(t, "eos", burns[0].Chain)
	assert.Equal(t, me, burns[0].Source)
	assert.Equal(t, uint64(20_000), burns[0].Amount)
	assert.Equal(t, uint64(508400100), burns[0].Height)

	assert.Equal(t, "burn02", burns[1].TxID)
	assert.Equal(t, uint64(15_000), burns[1].Amount, "both tokens of one transaction sum into one burn")
}

func TestDetectBurnsKeepsRawRowsVerbatim(t *testing.T) {
	window := chain.Window{Start: 508400000, End: 508500000}
	row := action(701, 508400100, "burn03", "core.vaulta", me, burnAccount, "2.0000 A", "hello")

	burns, err := eos.DetectBurns([]json.RawMessage{row}, burnAccount, window)
	require.NoError(t, err)
	require.Len(t, burns, 1)

	var raw []json.RawMessage
	require.NoError(t, json.Unmarshal(burns[0].Raw, &raw), "Raw is the JSON array of the burn's action rows")
	require.Len(t, raw, 1)
	assert.JSONEq(t, string(row), string(raw[0]))
}

func TestDetectBurnsFailsOnUninterpretableRow(t *testing.T) {
	_, err := eos.DetectBurns([]json.RawMessage{json.RawMessage(`{not json`)}, burnAccount, chain.Window{})
	assert.Error(t, err)
}
