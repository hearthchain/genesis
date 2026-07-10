package eos_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/chain/eos"
)

const me = "alicewyl1235"

// action renders one Hyperion get_actions row for a token transfer.
func action(seq, block uint64, trx, contract, from, to, quantity, memo string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"@timestamp":"2026-07-04T16:11:36.000","block_num":%d,"trx_id":%q,"global_sequence":%d,`+
			`"act":{"account":%q,"name":"transfer","data":{"from":%q,"to":%q,"quantity":%q,"memo":%q}}}`,
		block, trx, seq, contract, from, to, quantity, memo))
}

func TestDeltasNetsTransfersPerTransaction(t *testing.T) {
	rows := []json.RawMessage{
		action(101, 508400010, "aaaa01", "core.vaulta", "someoneelse1", me, "2.0000 A", ""),
		action(102, 508400020, "aaaa02", "eosio.token", me, "someoneelse1", "0.5000 EOS", "spend"),
	}

	deltas, status := eos.Deltas(rows, me)
	require.Equal(t, chain.StatusOK, status.Kind)
	require.Len(t, deltas, 2)
	assert.Equal(t, "aaaa01", deltas[0].TxID)
	assert.Equal(t, int64(20_000), deltas[0].Amount)
	assert.Equal(t, uint64(508400010), deltas[0].Height)
	assert.Equal(t, "aaaa02", deltas[1].TxID)
	assert.Equal(t, int64(-5_000), deltas[1].Amount)
}

func TestDeltasNetSwapToZero(t *testing.T) {
	// A core.vaulta swap: EOS out and A back within one transaction. The
	// combined balance does not move.
	rows := []json.RawMessage{
		action(201, 508400030, "swap01", "eosio.token", me, "core.vaulta", "5.0000 EOS", ""),
		action(202, 508400030, "swap01", "core.vaulta", "core.vaulta", me, "5.0000 A", ""),
	}

	deltas, status := eos.Deltas(rows, me)
	require.Equal(t, chain.StatusOK, status.Kind)
	require.Len(t, deltas, 1, "one delta per transaction")
	assert.Equal(t, int64(0), deltas[0].Amount)
}

func TestDeltasDeduplicateOverlappingPages(t *testing.T) {
	row := action(301, 508400040, "dup001", "core.vaulta", "someoneelse1", me, "1.0000 A", "")

	deltas, status := eos.Deltas([]json.RawMessage{row, row}, me)
	require.Equal(t, chain.StatusOK, status.Kind)
	require.Len(t, deltas, 1)
	assert.Equal(t, int64(10_000), deltas[0].Amount, "the same global sequence counts once")
}

func TestDeltasReadNormalizedAmountShape(t *testing.T) {
	// Hyperion sometimes normalizes transfer data into amount+symbol instead
	// of the original quantity string; parsing must stay integer-exact.
	row := json.RawMessage(`{"@timestamp":"2026-07-04T16:11:36.000","block_num":508400050,` +
		`"trx_id":"norm01","global_sequence":401,"act":{"account":"core.vaulta","name":"transfer",` +
		`"data":{"from":"someoneelse1","to":"` + me + `","amount":190.7,"symbol":"A","memo":""}}}`)

	deltas, status := eos.Deltas([]json.RawMessage{row}, me)
	require.Equal(t, chain.StatusOK, status.Kind)
	require.Len(t, deltas, 1)
	assert.Equal(t, int64(1_907_000), deltas[0].Amount)
}

func TestDeltasBlockOnUninterpretableRows(t *testing.T) {
	tests := []struct {
		name string
		row  json.RawMessage
	}{
		{"foreign contract", action(501, 508400060, "bad001", "wax.token", "someoneelse1", me, "1.0000 A", "")},
		{"non-transfer action", json.RawMessage(`{"@timestamp":"2026-07-04T16:11:36.000","block_num":508400061,` +
			`"trx_id":"bad002","global_sequence":502,"act":{"account":"core.vaulta","name":"swapto","data":{}}}`)},
		{"foreign symbol", action(503, 508400062, "bad003", "eosio.token", me, "someoneelse1", "1.0000 WAX", "")},
		{"undecodable", json.RawMessage(`{not json`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, status := eos.Deltas([]json.RawMessage{tc.row}, me)
			assert.Equal(t, chain.StatusUnsupported, status.Kind)
			assert.NotEmpty(t, status.Reason)
		})
	}
}
