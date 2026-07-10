package waves_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/genesis/internal/chain/waves"
)

const me = "3PMeMeMeMeMeMeMeMeMeMeMeMeMeMeMeMe1"

func rawTxs(t *testing.T, txs ...string) []json.RawMessage {
	t.Helper()
	out := make([]json.RawMessage, 0, len(txs))
	for _, s := range txs {
		require.True(t, json.Valid([]byte(s)), s)
		out = append(out, json.RawMessage(s))
	}
	return out
}

func TestDeltasTransferRules(t *testing.T) {
	txs := rawTxs(t,
		// incoming WAVES transfer: +amount
		`{"type":4,"id":"in1","sender":"3POther","recipient":"`+me+`","assetId":null,"amount":1000,"fee":5,"feeAssetId":null,"timestamp":1000,"height":100}`,
		// outgoing WAVES transfer: -amount-fee
		`{"type":4,"id":"out1","sender":"`+me+`","recipient":"3POther","assetId":null,"amount":300,"fee":5,"feeAssetId":null,"timestamp":2000,"height":200}`,
		// outgoing token transfer with WAVES fee: fee only
		`{"type":4,"id":"tok1","sender":"`+me+`","recipient":"3POther","assetId":"AssetXYZ","amount":77,"fee":5,"feeAssetId":null,"timestamp":3000,"height":300}`,
		// outgoing token transfer with sponsored (non-WAVES) fee: zero delta
		`{"type":4,"id":"tok2","sender":"`+me+`","recipient":"3POther","assetId":"AssetXYZ","amount":77,"fee":5,"feeAssetId":"AssetFee","timestamp":4000,"height":400}`,
		// self-send WAVES: only the fee leaves
		`{"type":4,"id":"self1","sender":"`+me+`","recipient":"`+me+`","assetId":null,"amount":500,"fee":5,"feeAssetId":null,"timestamp":5000,"height":500}`,
	)
	deltas, status := waves.Deltas(txs, me)
	require.Equal(t, "ok", status.Kind)

	got := map[string]int64{}
	for _, d := range deltas {
		got[d.TxID] = d.Amount
	}
	assert.Equal(t, int64(1000), got["in1"])
	assert.Equal(t, int64(-305), got["out1"])
	assert.Equal(t, int64(-5), got["tok1"])
	assert.Equal(t, int64(0), got["tok2"])
	assert.Equal(t, int64(-5), got["self1"])
}

func TestDeltasGenesisPaymentAndMassTransfer(t *testing.T) {
	txs := rawTxs(t,
		`{"type":1,"id":"gen1","recipient":"`+me+`","amount":10000,"fee":0,"timestamp":1000,"height":1}`,
		`{"type":2,"id":"pay1","sender":"3POther","recipient":"`+me+`","amount":2000,"fee":1,"timestamp":2000,"height":50}`,
		`{"type":2,"id":"pay2","sender":"`+me+`","recipient":"3POther","amount":500,"fee":1,"timestamp":3000,"height":60}`,
		// incoming mass transfer: two entries to me
		`{"type":11,"id":"massin","sender":"3POther","assetId":null,"fee":2,"timestamp":4000,"height":70,"transfers":[{"recipient":"`+me+`","amount":100},{"recipient":"3PElse","amount":50},{"recipient":"`+me+`","amount":200}]}`,
		// outgoing mass transfer including an entry back to self
		`{"type":11,"id":"massout","sender":"`+me+`","assetId":null,"fee":2,"timestamp":5000,"height":80,"transfers":[{"recipient":"3PElse","amount":400},{"recipient":"`+me+`","amount":100}]}`,
		// outgoing token mass transfer: fee only
		`{"type":11,"id":"masstok","sender":"`+me+`","assetId":"AssetXYZ","fee":2,"timestamp":6000,"height":90,"transfers":[{"recipient":"3PElse","amount":400}]}`,
	)
	deltas, status := waves.Deltas(txs, me)
	require.Equal(t, "ok", status.Kind)

	got := map[string]int64{}
	for _, d := range deltas {
		got[d.TxID] = d.Amount
	}
	assert.Equal(t, int64(10000), got["gen1"])
	assert.Equal(t, int64(2000), got["pay1"])
	assert.Equal(t, int64(-501), got["pay2"])
	assert.Equal(t, int64(300), got["massin"])
	assert.Equal(t, int64(-402), got["massout"], "-400 to others -2 fee; the self entry returns")
	assert.Equal(t, int64(-2), got["masstok"])
}

func TestDeltasUnsupportedTypeBlocksTheAddress(t *testing.T) {
	txs := rawTxs(t,
		`{"type":4,"id":"in1","sender":"3POther","recipient":"`+me+`","assetId":null,"amount":1000,"fee":5,"feeAssetId":null,"timestamp":1000,"height":100}`,
		`{"type":8,"id":"lease1","sender":"`+me+`","recipient":"3POther","amount":100,"fee":5,"timestamp":2000,"height":200}`,
	)
	_, status := waves.Deltas(txs, me)
	assert.Equal(t, "unsupported", status.Kind)
	assert.Contains(t, status.Reason, "type 8")
}

func TestDeltasAreHeightAscending(t *testing.T) {
	txs := rawTxs(t,
		`{"type":4,"id":"late","sender":"`+me+`","recipient":"3POther","assetId":null,"amount":10,"fee":1,"feeAssetId":null,"timestamp":9000,"height":900}`,
		`{"type":4,"id":"early","sender":"3POther","recipient":"`+me+`","assetId":null,"amount":100,"fee":1,"feeAssetId":null,"timestamp":1000,"height":100}`,
	)
	deltas, status := waves.Deltas(txs, me)
	require.Equal(t, "ok", status.Kind)
	require.Len(t, deltas, 2)
	assert.Equal(t, "early", deltas[0].TxID)
	assert.Equal(t, "late", deltas[1].TxID)
}
