package eos_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/genesis/internal/chain/eos"
)

func greymassServer(t *testing.T, payload []byte) *eos.Greymass {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/history/get_transaction", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return eos.NewGreymass(srv.URL)
}

func TestGreymassReadsCanonicalTransferTraces(t *testing.T) {
	g := greymassServer(t, fixture(t, "greymass_tx.json"))

	info, err := g.GetTransaction(t.Context(), "3e2581376b6e2a3705d7225a7a6889e5afe0fc5578cc2d27b8eeb8554abd6631")
	require.NoError(t, err)

	assert.Equal(t, "3e2581376b6e2a3705d7225a7a6889e5afe0fc5578cc2d27b8eeb8554abd6631", info.ID)
	assert.Equal(t, uint64(507709376), info.BlockNum)
	assert.True(t, info.Irreversible)

	// Each action appears once per notified receiver; only the contract's own
	// execution trace counts, and foreign-token transfers are dropped.
	require.Len(t, info.Transfers, 4)
	burn := info.Transfers[1]
	assert.Equal(t, "core.vaulta", burn.Contract)
	assert.Equal(t, "alicewyl1235", burn.From)
	assert.Equal(t, "eosio.null", burn.To)
	assert.Equal(t, "0.0100 A", burn.Quantity)
	assert.Equal(t, "minepool burn", burn.Memo)
}
