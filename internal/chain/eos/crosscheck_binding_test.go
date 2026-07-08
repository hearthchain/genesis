package eos_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/chain"
	"github.com/hearthchain/burning-page/internal/chain/eos"
)

// greymassWithMemo rewrites the recorded transaction's eosio.null trace memo.
func greymassWithMemo(t *testing.T, memo string, irreversible bool) []byte {
	t.Helper()
	var tx map[string]any
	require.NoError(t, json.Unmarshal(fixture(t, "greymass_tx.json"), &tx))
	tx["irreversible"] = irreversible
	traces, ok := tx["traces"].([]any)
	require.True(t, ok)
	for _, raw := range traces {
		trace, isMap := raw.(map[string]any)
		require.True(t, isMap)
		act, isMap := trace["act"].(map[string]any)
		require.True(t, isMap)
		data, isMap := act["data"].(map[string]any)
		require.True(t, isMap)
		if data["to"] == "eosio.null" {
			data["memo"] = memo
		}
	}
	payload, err := json.Marshal(tx)
	require.NoError(t, err)
	return payload
}

func recordedMemoBinding(t *testing.T, hearth string) chain.MemoBinding {
	t.Helper()
	return chain.MemoBinding{
		Source: "alicewyl1235",
		Hearth: hearth,
		TxID:   recordedTrx,
		Height: 507709376,
	}
}

func TestCrossCheckBindingConfirmsMatchingMemo(t *testing.T) {
	hearth := hearthAddress(t, "eos binding test")
	g := greymassServer(t, greymassWithMemo(t, bindingMemo("alicewyl1235", hearth), true))

	verdict, err := eos.CrossCheckBinding(t.Context(), g, recordedMemoBinding(t, hearth))
	require.NoError(t, err)
	assert.Equal(t, "confirmed", verdict.Status)
}

func TestCrossCheckBindingPendsWhileReversible(t *testing.T) {
	hearth := hearthAddress(t, "eos binding test")
	g := greymassServer(t, greymassWithMemo(t, bindingMemo("alicewyl1235", hearth), false))

	verdict, err := eos.CrossCheckBinding(t.Context(), g, recordedMemoBinding(t, hearth))
	require.NoError(t, err)
	assert.Equal(t, "pending_crosscheck", verdict.Status)
}

func TestCrossCheckBindingFlagsAbsentMemo(t *testing.T) {
	hearth := hearthAddress(t, "eos binding test")
	g := greymassServer(t, fixture(t, "greymass_tx.json")) // memo stays "minepool burn"

	verdict, err := eos.CrossCheckBinding(t.Context(), g, recordedMemoBinding(t, hearth))
	require.NoError(t, err)
	assert.Equal(t, "mismatch", verdict.Status)
	assert.Contains(t, verdict.Mismatches, "memo")
}
