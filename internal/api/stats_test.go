package api_test

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/genesis/internal/store"
)

func wavesStats(t *testing.T, stats map[string]any) map[string]any {
	t.Helper()
	chains, ok := stats["chains"].(map[string]any)
	require.True(t, ok)
	waves, ok := chains["waves"].(map[string]any)
	require.True(t, ok)
	return waves
}

func TestStatsAggregatesCreditedBurns(t *testing.T) {
	id := newIdentity(t, "api test burner")
	srv := newServer(t, id)
	require.Equal(t, http.StatusCreated, postBinding(t, srv.URL, id, id.sig).StatusCode)

	got := getJSON(t, srv.URL+"/api/stats")
	assert.Equal(t, "49713174000", got["totalCreditMicro"])
	assert.Equal(t, "49713.174000", got["totalCredit"])
	assert.NotEmpty(t, got["merkleRoot"])
	assert.Equal(t, float64(1), got["participants"])
	assert.Equal(t, float64(1), got["bindings"])
	assert.Equal(t, float64(0), got["pendingSources"])
	assert.Equal(t, float64(0), got["blockedSources"])

	waves := wavesStats(t, got)
	assert.Equal(t, "100000000000", waves["burnedBaseUnits"])
	assert.Equal(t, "0", waves["pendingBaseUnits"])
	byStatus, ok := waves["burnsByStatus"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), byStatus["confirmed"])

	windows, ok := got["windows"].(map[string]any)
	require.True(t, ok)
	window, ok := windows["waves"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(4000000), window["startHeight"])
	assert.Equal(t, float64(4001000), window["endHeight"])
}

func TestStatsCountsPendingBurnsSeparately(t *testing.T) {
	id := newIdentity(t, "api test burner")
	srv := newServer(t, id)
	require.Equal(t, http.StatusCreated, postBinding(t, srv.URL, id, id.sig).StatusCode)

	require.NoError(t, store.AppendJSONL(filepath.Join(serverDataDir, "burns.jsonl"), map[string]any{
		"txId": "Fresh1", "chain": "waves", "source": id.source, "amountBaseUnits": 10000000,
		"height": 4000900, "timestamp": "2026-08-01T13:00:00Z", "status": "pending_confirmations",
	}))

	got := getJSON(t, srv.URL+"/api/stats")
	assert.Equal(t, "49713174000", got["totalCreditMicro"], "pending burns add no credit")

	waves := wavesStats(t, got)
	assert.Equal(t, "100000000000", waves["burnedBaseUnits"])
	assert.Equal(t, "10000000", waves["pendingBaseUnits"])
	byStatus, ok := waves["burnsByStatus"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), byStatus["confirmed"])
	assert.Equal(t, float64(1), byStatus["pending_confirmations"])
}

func TestStatsWithoutBindingShowsPendingSource(t *testing.T) {
	id := newIdentity(t, "api test burner")
	srv := newServer(t, id)

	got := getJSON(t, srv.URL+"/api/stats")
	assert.Equal(t, "0", got["totalCreditMicro"])
	assert.Equal(t, float64(0), got["participants"])
	assert.Equal(t, float64(0), got["bindings"])
	assert.Equal(t, float64(1), got["pendingSources"], "the burn exists but its source is unbound")
}

func TestStatsDeduplicatesSupersededBurnRows(t *testing.T) {
	id := newIdentity(t, "api test burner")
	srv := newServer(t, id)
	require.Equal(t, http.StatusCreated, postBinding(t, srv.URL, id, id.sig).StatusCode)

	// The watcher appends a status update for an already-recorded burn: the
	// latest row per txId wins, the amount is counted once.
	require.NoError(t, store.AppendJSONL(filepath.Join(serverDataDir, "burns.jsonl"), map[string]any{
		"txId": "B1", "chain": "waves", "source": id.source, "amountBaseUnits": 100000000000,
		"height": 4000010, "timestamp": "2026-08-01T12:00:00Z", "status": "confirmed",
	}))

	got := getJSON(t, srv.URL+"/api/stats")
	waves := wavesStats(t, got)
	assert.Equal(t, "100000000000", waves["burnedBaseUnits"])
	byStatus, ok := waves["burnsByStatus"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), byStatus["confirmed"])
}
