package eos_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/chain"
	"github.com/hearthchain/burning-page/internal/chain/eos"
)

// recordedAdapter wires the adapter over one httptest server replaying the
// recorded chain-API, Hyperion and Greymass responses.
func recordedAdapter(t *testing.T) *eos.Adapter {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chain/get_info", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture(t, "get_info.json"))
	})
	mux.HandleFunc("/v1/chain/get_currency_balance", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Code string `json:"code"`
		}
		require.NoError(t, jsonDecode(r, &req))
		if req.Code == "core.vaulta" {
			_, _ = w.Write(fixture(t, "balance_a.json"))
			return
		}
		_, _ = w.Write(fixture(t, "balance_eos.json"))
	})
	mux.HandleFunc("/v1/chain/get_account", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture(t, "get_account.json"))
	})
	mux.HandleFunc("/v2/history/get_actions", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("skip") {
		case "", "0":
			_, _ = w.Write(fixture(t, "actions_page1.json"))
		case "3":
			_, _ = w.Write(fixture(t, "actions_page2.json"))
		default:
			_, _ = w.Write([]byte(`{"total":{"value":406},"actions":[]}`))
		}
	})
	mux.HandleFunc("/v1/history/get_transaction", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture(t, "greymass_tx.json"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &eos.Adapter{
		API:          eos.NewClient(srv.URL),
		Index:        eos.NewHyperion(srv.URL, eos.WithHyperionPageLimit(3)),
		Secondary:    eos.NewGreymass(srv.URL),
		BurnAccount:  "eosio.null",
		HearthScheme: 'H',
	}
}

func TestAdapterSatisfiesTheChainPort(t *testing.T) {
	a := recordedAdapter(t)

	var _ chain.Adapter = a
	var _ chain.BindingSource = a
	assert.Equal(t, "eos", a.Name())
	assert.NoError(t, a.ValidateAddress("alicewyl1235"))
	assert.Error(t, a.ValidateAddress("NotAnAccount"))
}

func TestAdapterHeightIsTheLastIrreversibleBlock(t *testing.T) {
	a := recordedAdapter(t)

	h, err := a.Height(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint64(508445489), h)
}

func TestAdapterDetectsRecordedBurns(t *testing.T) {
	a := recordedAdapter(t)

	burns, err := a.BurnCandidates(t.Context(), chain.Window{Start: 467687501, End: 508500000})
	require.NoError(t, err)
	require.Len(t, burns, 6, "every recorded eosio.null transfer is inside the window")
	assert.Equal(t, "eos", burns[0].Chain)
	assert.Equal(t, "vaulta", burns[0].Source)
	assert.Equal(t, uint64(1), burns[0].Amount)

	narrower, err := a.BurnCandidates(t.Context(), chain.Window{Start: 504000000, End: 506000000})
	require.NoError(t, err)
	assert.Len(t, narrower, 3)
}

func TestAdapterHistoryRoundTripsThroughDeltas(t *testing.T) {
	a := recordedAdapter(t)

	h, err := a.History(t.Context(), "eosio.null", 508445489, 508445489)
	require.NoError(t, err)
	require.Equal(t, chain.StatusOK, h.Status)
	assert.Equal(t, uint64(1_907_113+5_311_067), h.NodeBalance)
	assert.Equal(t, int64(378_663), h.Recomputed, "the six recorded incoming transfers")
	assert.Equal(t, uint64(1_907_113+5_311_067-378_663), h.OpeningBaseUnits)

	deltas, status := a.Deltas(h.Txs, "eosio.null")
	require.Equal(t, chain.StatusOK, status.Kind)
	var sum int64
	for _, d := range deltas {
		sum += d.Amount
	}
	assert.Equal(t, h.Recomputed, sum, "Deltas over History.Txs reproduces the recomputed sum")
}

func TestAdapterCrossChecksViaTheSecondary(t *testing.T) {
	a := recordedAdapter(t)

	verdict, err := a.CrossCheck(t.Context(), recordedBurn(t), 0)
	require.NoError(t, err)
	assert.Equal(t, "confirmed", verdict.Status)
}

func TestAdapterMemoBindingsIgnorePlainMemos(t *testing.T) {
	a := recordedAdapter(t)

	bindings, err := a.MemoBindings(t.Context(), 0)
	require.NoError(t, err)
	assert.Empty(t, bindings, "none of the recorded memos is a binding statement")
}
