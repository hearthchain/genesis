package eos_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/genesis/internal/chain/eos"
)

// hyperionServer replays the two recorded eosio.null pages (3 actions each,
// total 406) for any skip beyond them it returns an empty page.
func hyperionServer(t *testing.T) (*eos.Hyperion, *int) {
	t.Helper()
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/history/get_actions", func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "asc", r.URL.Query().Get("sort"), "pages must come in ascending chain order")
		require.NotEmpty(t, r.URL.Query().Get("filter"))
		switch r.URL.Query().Get("skip") {
		case "", "0":
			_, _ = w.Write(fixture(t, "actions_page1.json"))
		case "3":
			_, _ = w.Write(fixture(t, "actions_page2.json"))
		default:
			_, _ = w.Write([]byte(`{"total":{"value":406},"actions":[]}`))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return eos.NewHyperion(srv.URL, eos.WithHyperionPageLimit(3)), &calls
}

func TestHyperionPaginatesTransferActions(t *testing.T) {
	h, _ := hyperionServer(t)

	rows, err := h.TransfersTo(t.Context(), "eosio.null", 500)
	require.NoError(t, err)
	require.Len(t, rows, 6, "two full pages then an empty one")

	var first struct {
		TrxID string `json:"trx_id"`
		Act   struct {
			Data struct {
				To string `json:"to"`
			} `json:"data"`
		} `json:"act"`
	}
	require.NoError(t, json.Unmarshal(rows[0], &first))
	assert.Equal(t, "15d70489b7b47e43cabb22a4f02ed2a1dd128969f871ce9d983acd91e3fd41df", first.TrxID)
	assert.Equal(t, "eosio.null", first.Act.Data.To)
}

func TestHyperionRefusesOversizedHistories(t *testing.T) {
	h, calls := hyperionServer(t)

	_, err := h.TransferActions(t.Context(), "eosio.null", 100)
	require.Error(t, err, "total 406 exceeds the 100-action cap")
	assert.ErrorIs(t, err, eos.ErrHistoryTooLarge)
	assert.Equal(t, 1, *calls, "the cap check happens on the first page")
}
