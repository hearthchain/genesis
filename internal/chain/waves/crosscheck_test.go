package waves_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/chain"
	"github.com/hearthchain/burning-page/internal/chain/waves"
)

func secondaryNode(t *testing.T, height uint64, txJSON string) *waves.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/blocks/height", func(w http.ResponseWriter, _ *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(map[string]uint64{"height": height}))
	})
	mux.HandleFunc("/transactions/info/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(txJSON))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return waves.NewClient(srv.URL)
}

func fixtureBurn(t *testing.T) (chain.Burn, string) {
	t.Helper()
	burns, err := waves.DetectBurns(burnFixtureTxs(t), burnAddr, chain.Window{Start: 4000000, End: 4001000})
	require.NoError(t, err)
	require.NotEmpty(t, burns)
	return burns[0], string(burns[0].Raw)
}

func TestCrossCheckConfirmsMatchingTx(t *testing.T) {
	burn, raw := fixtureBurn(t)
	secondary := secondaryNode(t, burn.Height+200, raw)

	verdict, err := waves.CrossCheck(t.Context(), secondary, burn, 100)
	require.NoError(t, err)
	assert.Equal(t, "confirmed", verdict.Status)
	assert.Empty(t, verdict.Mismatches)
}

func TestCrossCheckPendsWhileSecondaryLags(t *testing.T) {
	burn, raw := fixtureBurn(t)
	secondary := secondaryNode(t, burn.Height+50, raw) // fewer than 100 confirmations on node B

	verdict, err := waves.CrossCheck(t.Context(), secondary, burn, 100)
	require.NoError(t, err)
	assert.Equal(t, "pending_crosscheck", verdict.Status)
}

func TestCrossCheckFlagsDivergingAmount(t *testing.T) {
	burn, raw := fixtureBurn(t)
	var tampered map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &tampered))
	tampered["amount"] = 999999999
	tamperedJSON, err := json.Marshal(tampered)
	require.NoError(t, err)
	secondary := secondaryNode(t, burn.Height+200, string(tamperedJSON))

	verdict, err := waves.CrossCheck(t.Context(), secondary, burn, 100)
	require.NoError(t, err)
	assert.Equal(t, "mismatch", verdict.Status)
	assert.Contains(t, verdict.Mismatches, "amount")
}
