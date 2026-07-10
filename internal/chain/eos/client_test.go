package eos_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/chain/eos"
)

func jsonDecode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return raw
}

// chainAPIServer replays the recorded EOS Nation chain-API responses.
func chainAPIServer(t *testing.T) *eos.Client {
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
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return eos.NewClient(srv.URL)
}

func TestClientReadsLastIrreversibleBlock(t *testing.T) {
	c := chainAPIServer(t)

	lib, err := c.LastIrreversibleBlock(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint64(508445489), lib)
}

func TestClientSumsCombinedLiquidBalance(t *testing.T) {
	c := chainAPIServer(t)

	balance, err := c.CombinedBalance(t.Context(), "eosio.null")
	require.NoError(t, err)
	assert.Equal(t, uint64(1_907_113+5_311_067), balance, "190.7113 A plus 531.1067 EOS in base units")
}

func TestClientReadsAccountCreationTime(t *testing.T) {
	c := chainAPIServer(t)

	created, err := c.AccountCreated(t.Context(), "eosio.null")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2018, 6, 8, 8, 8, 8, 500_000_000, time.UTC), created)
}
