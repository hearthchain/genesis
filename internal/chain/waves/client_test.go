package waves_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/chain/waves"
)

const fixtureAddr = "3PQwxpPWEsHYiFnrncQJNvLmrAXxR454vFy"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return b
}

func lastTxID(t *testing.T, page []byte) string {
	t.Helper()
	var outer []json.RawMessage
	require.NoError(t, json.Unmarshal(page, &outer))
	var txs []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(outer[0], &txs))
	require.NotEmpty(t, txs)
	return txs[len(txs)-1].ID
}

// fixtureNode replays the recorded mainnet responses keyed by path + after cursor.
func fixtureNode(t *testing.T) *httptest.Server {
	t.Helper()
	page1 := fixture(t, "history_page1.json")
	page2 := fixture(t, "history_page2_after.json")
	afterPage1 := lastTxID(t, page1)
	afterPage2 := lastTxID(t, page2)

	mux := http.NewServeMux()
	mux.HandleFunc("/transactions/address/"+fixtureAddr+"/limit/100", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("after") {
		case "":
			_, _ = w.Write(page1)
		case afterPage1:
			_, _ = w.Write(page2)
		case afterPage2:
			_, _ = w.Write([]byte("[[]]"))
		default:
			http.Error(w, "unexpected cursor", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/blocks/height", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture(t, "height.json"))
	})
	mux.HandleFunc("/addresses/balance/"+fixtureAddr+"/100", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture(t, "balance_confirmations.json"))
	})
	mux.HandleFunc("/transactions/info/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fixture(t, "txinfo_secondary.json"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAllTransactionsFollowsAfterCursorToTheEnd(t *testing.T) {
	srv := fixtureNode(t)
	c := waves.NewClient(srv.URL, waves.WithPageLimit(100))

	txs, err := c.AllTransactions(t.Context(), fixtureAddr)
	require.NoError(t, err)
	assert.Len(t, txs, 200, "two full fixture pages, then an empty one")

	var first struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(txs[0], &first))
	assert.Equal(t, "DBF6yG8xtDPXNeVGBfYVcqXyBmRZ9X1oEXq3qjSYkP5R", first.ID, "order preserved from the node")
}

func TestHeightBalanceAndTxInfoGoldenValues(t *testing.T) {
	srv := fixtureNode(t)
	c := waves.NewClient(srv.URL, waves.WithPageLimit(100))

	height, err := c.Height(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint64(5300984), height)

	balance, err := c.BalanceAfterConfirmations(t.Context(), fixtureAddr, 100)
	require.NoError(t, err)
	assert.Positive(t, balance)

	info, err := c.TransactionInfo(t.Context(), "DBF6yG8xtDPXNeVGBfYVcqXyBmRZ9X1oEXq3qjSYkP5R")
	require.NoError(t, err)
	var tx struct {
		Height uint64 `json:"height"`
	}
	require.NoError(t, json.Unmarshal(info, &tx))
	assert.Positive(t, tx.Height)
}

func TestClientRetriesOnceOnServerError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"height":42}`))
	}))
	t.Cleanup(srv.Close)

	c := waves.NewClient(srv.URL)
	height, err := c.Height(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint64(42), height)
	assert.Equal(t, int32(2), calls.Load())
}
