package watcher_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/bindings"
	"github.com/hearthchain/burning-page/internal/chain"
	"github.com/hearthchain/burning-page/internal/chain/chains"
	"github.com/hearthchain/burning-page/internal/config"
	"github.com/hearthchain/burning-page/internal/journal"
	"github.com/hearthchain/burning-page/internal/snapshot"
	"github.com/hearthchain/burning-page/internal/store"
	"github.com/hearthchain/burning-page/internal/watcher"
)

// TestEosFixtureEndToEnd drives the whole EOS pipeline offline: the fixture
// adapter feeds one burn, one dust memo binding, one swap and one plain
// spend; the poll confirms the burn, records the history with its synthetic
// opening layer and harvests the binding; the snapshot prices it all.
func TestEosFixtureEndToEnd(t *testing.T) {
	const source = "alicewyl1235"
	const hearth = "3HGBPbaEvujdeikDNDwQ9gsZBVb3hY3HE2j"
	dataDir := t.TempDir()

	var cc config.ChainConfig
	cc.BurnAddress = "eosio.null"
	cc.Window = chain.Window{Start: 508400000, End: 508500000}
	adapter, err := chains.NewFixture("eos", "../../fixtures/e2e/eos", cc, 'H')
	require.NoError(t, err)
	reg, err := bindings.Load(filepath.Join(dataDir, "bindings.jsonl"), 'H')
	require.NoError(t, err)
	w := &watcher.Watcher{Adapter: adapter, ChainCfg: cc, DataDir: dataDir, Registry: reg}

	require.NoError(t, w.Poll(t.Context()))

	records, err := store.ReadJSONL[watcher.BurnRecord](filepath.Join(dataDir, "burns.jsonl"))
	require.NoError(t, err)
	statuses := map[string]string{}
	for _, r := range records {
		statuses[r.TxID] = r.Status
	}
	assert.Equal(t, "confirmed", statuses["eosburn0001"])
	assert.Equal(t, "confirmed", statuses["eosbind0001"], "the dust binding transfer is a burn too")

	meta, _, err := store.ReadTransfers(filepath.Join(dataDir, "transfers", "eos", source+".jsonl"))
	require.NoError(t, err)
	assert.Equal(t, "ok", meta.Status)
	assert.Equal(t, uint64(10_000), meta.OpeningBaseUnits, "1.0000 A predates the public index")

	bound, ok := reg.HearthFor(source)
	require.True(t, ok, "the memo binding was harvested")
	assert.Equal(t, hearth, bound)

	j, err := journal.Load("../../data/journal/eos.csv")
	require.NoError(t, err)
	snap, bundles, err := snapshot.Build(dataDir, map[string]*journal.Journal{"eos": j}, 'H')
	require.NoError(t, err)
	require.Len(t, snap.Entries, 1)
	assert.Equal(t, hearth, snap.Entries[0].Hearth)
	assert.NotEqual(t, "0", snap.Entries[0].CreditMicro, "the burn earns a nonzero floor credit")
	assert.Empty(t, snap.PendingSources)
	assert.Empty(t, snap.BlockedSources)
	assert.Len(t, bundles, 2)
}
