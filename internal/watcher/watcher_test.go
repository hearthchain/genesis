package watcher_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wavesplatform/gowaves/pkg/crypto"

	"github.com/hearthchain/genesis/internal/bindings"
	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/chain/waves"
	"github.com/hearthchain/genesis/internal/config"
	"github.com/hearthchain/genesis/internal/hearthaddr"
	"github.com/hearthchain/genesis/internal/store"
	"github.com/hearthchain/genesis/internal/watcher"
)

const (
	burnAddr = "3PHearthBurnXXXXXXXXXXXXXXXXXZgJXd1"
	alice    = "3PSenderAlice1111111111111111111111"
	carol    = "3PSenderCarol333333333333333333333"
)

// fakeNode serves canned histories, a fixed tip and per-address balances.
type fakeNode struct {
	histories map[string][]json.RawMessage
	byID      map[string]json.RawMessage
	tip       uint64
	balances  map[string]uint64
}

func (f *fakeNode) AllTransactions(_ context.Context, addr string) ([]json.RawMessage, error) {
	txs, ok := f.histories[addr]
	if !ok {
		return nil, fmt.Errorf("fake: unknown address %s", addr)
	}
	return txs, nil
}

func (f *fakeNode) Height(context.Context) (uint64, error) { return f.tip, nil }

func (f *fakeNode) BalanceAfterConfirmations(_ context.Context, addr string, _ uint64) (uint64, error) {
	return f.balances[addr], nil
}

func (f *fakeNode) TransactionInfo(_ context.Context, id string) (json.RawMessage, error) {
	tx, ok := f.byID[id]
	if !ok {
		return nil, fmt.Errorf("fake: unknown tx %s", id)
	}
	return tx, nil
}

func tx(s string) json.RawMessage { return json.RawMessage(s) }

func testNode(t *testing.T) *fakeNode {
	t.Helper()
	// Alice: deposit 2 WAVES at h4000001, burns 1 WAVES (fee 0.001) at h4000010.
	aliceDeposit := tx(`{"type":4,"id":"AliceDeposit1","sender":"3POther","recipient":"` + alice + `","assetId":null,"amount":200000000,"fee":100000,"feeAssetId":null,"timestamp":1753900000000,"height":4000001}`)
	aliceBurn := tx(`{"type":4,"id":"AliceBurn1111","sender":"` + alice + `","recipient":"` + burnAddr + `","assetId":null,"amount":100000000,"fee":100000,"feeAssetId":null,"timestamp":1754006400000,"height":4000010,"applicationStatus":"succeeded"}`)
	// Carol: history contains a lease -> unsupported.
	carolDeposit := tx(`{"type":4,"id":"CarolDeposit1","sender":"3POther","recipient":"` + carol + `","assetId":null,"amount":900000000,"fee":100000,"feeAssetId":null,"timestamp":1753900000000,"height":4000002}`)
	carolLease := tx(`{"type":8,"id":"CarolLease111","sender":"` + carol + `","recipient":"3POther","amount":100,"fee":100000,"timestamp":1753900001000,"height":4000003}`)
	carolBurn := tx(`{"type":4,"id":"CarolBurn1111","sender":"` + carol + `","recipient":"` + burnAddr + `","assetId":null,"amount":50000000,"fee":100000,"feeAssetId":null,"timestamp":1754006401000,"height":4000011,"applicationStatus":"succeeded"}`)
	freshBurn := tx(`{"type":4,"id":"FreshBurn9999","sender":"` + alice + `","recipient":"` + burnAddr + `","assetId":null,"amount":10000000,"fee":100000,"feeAssetId":null,"timestamp":1754010000000,"height":4000150,"applicationStatus":"succeeded"}`)

	return &fakeNode{
		histories: map[string][]json.RawMessage{
			burnAddr: {aliceBurn, carolBurn, freshBurn},
			alice:    {aliceBurn, aliceDeposit},
			carol:    {carolBurn, carolLease, carolDeposit},
		},
		byID: map[string]json.RawMessage{
			"AliceBurn1111": aliceBurn,
			"CarolBurn1111": carolBurn,
			"FreshBurn9999": freshBurn,
		},
		tip: 4000200,
		balances: map[string]uint64{
			alice: 99900000, // 2 WAVES - 1 WAVES burned - 0.001 fee
			carol: 849800000,
		},
	}
}

func testAdapter(node *fakeNode) *waves.Adapter {
	return &waves.Adapter{Primary: node, Secondary: node, BurnAddress: burnAddr}
}

func testChainConfig() config.ChainConfig {
	var cc config.ChainConfig
	cc.BurnAddress = burnAddr
	cc.Window = chain.Window{Start: 4000000, End: 4001000}
	cc.Confirmations = 100
	return cc
}

func TestPollDetectsCrossChecksAndWritesArtifacts(t *testing.T) {
	node := testNode(t)
	dataDir := t.TempDir()
	w := &watcher.Watcher{Adapter: testAdapter(node), ChainCfg: testChainConfig(), DataDir: dataDir}

	require.NoError(t, w.Poll(t.Context()))

	records, err := store.ReadJSONL[watcher.BurnRecord](filepath.Join(dataDir, "burns.jsonl"))
	require.NoError(t, err)
	require.Len(t, records, 3)
	byID := map[string]watcher.BurnRecord{}
	for _, r := range records {
		byID[r.TxID] = r
	}
	assert.Equal(t, "confirmed", byID["AliceBurn1111"].Status)
	assert.Equal(t, uint64(100000000), byID["AliceBurn1111"].Amount)
	assert.Equal(t, "confirmed", byID["CarolBurn1111"].Status)
	assert.Equal(t, "pending_confirmations", byID["FreshBurn9999"].Status,
		"a fresh burn is visible immediately, credit waits for maturity")

	aliceMeta, _, err := store.ReadTransfers(filepath.Join(dataDir, "transfers", "waves", alice+".jsonl"))
	require.NoError(t, err)
	assert.Equal(t, "ok", aliceMeta.Status)
	assert.Equal(t, "waves", aliceMeta.Chain)
	assert.Equal(t, int64(99900000), aliceMeta.Recomputed)
	assert.Equal(t, uint64(99900000), aliceMeta.NodeBalance)

	carolMeta, _, err := store.ReadTransfers(filepath.Join(dataDir, "transfers", "waves", carol+".jsonl"))
	require.NoError(t, err)
	assert.Equal(t, "unsupported", carolMeta.Status)
	assert.Contains(t, carolMeta.Reason, "type 8")
}

func TestPollIsIdempotentAcrossRestarts(t *testing.T) {
	node := testNode(t)
	dataDir := t.TempDir()

	w := &watcher.Watcher{Adapter: testAdapter(node), ChainCfg: testChainConfig(), DataDir: dataDir}
	require.NoError(t, w.Poll(t.Context()))

	// A fresh watcher over the same data dir must not duplicate anything.
	w2 := &watcher.Watcher{Adapter: testAdapter(node), ChainCfg: testChainConfig(), DataDir: dataDir}
	require.NoError(t, w2.Poll(t.Context()))

	records, err := store.ReadJSONL[watcher.BurnRecord](filepath.Join(dataDir, "burns.jsonl"))
	require.NoError(t, err)
	assert.Len(t, records, 3, "rescan must skip already-recorded states")

	entries, err := os.ReadDir(filepath.Join(dataDir, "transfers", "waves"))
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestPendingBurnUpgradesWhenMature(t *testing.T) {
	node := testNode(t)
	dataDir := t.TempDir()
	w := &watcher.Watcher{Adapter: testAdapter(node), ChainCfg: testChainConfig(), DataDir: dataDir}
	require.NoError(t, w.Poll(t.Context()))

	node.tip = 4000300 // FreshBurn9999 at 4000150 now has >100 confirmations
	require.NoError(t, w.Poll(t.Context()))

	records, err := store.ReadJSONL[watcher.BurnRecord](filepath.Join(dataDir, "burns.jsonl"))
	require.NoError(t, err)
	latest := map[string]string{}
	for _, r := range records {
		latest[r.TxID] = r.Status
	}
	assert.Equal(t, "confirmed", latest["FreshBurn9999"], "the superseding record wins")
}

// fakeBindingAdapter is a chain.Adapter + chain.BindingSource with scripted
// memo bindings and per-tx cross-check verdicts.
type fakeBindingAdapter struct {
	tip      uint64
	memos    []chain.MemoBinding
	verdicts map[string]string
}

func (f *fakeBindingAdapter) Name() string                           { return "eos" }
func (f *fakeBindingAdapter) ValidateAddress(string) error           { return nil }
func (f *fakeBindingAdapter) Height(context.Context) (uint64, error) { return f.tip, nil }

func (f *fakeBindingAdapter) BurnCandidates(context.Context, chain.Window) ([]chain.Burn, error) {
	return nil, nil
}

func (f *fakeBindingAdapter) CrossCheck(context.Context, chain.Burn, uint64) (chain.Verdict, error) {
	return chain.Verdict{Status: "confirmed"}, nil
}

func (f *fakeBindingAdapter) History(_ context.Context, source string, reference, _ uint64) (chain.History, error) {
	return chain.History{Address: source, ReferenceHeight: reference, Status: "ok"}, nil
}

func (f *fakeBindingAdapter) Deltas([]json.RawMessage, string) ([]chain.Delta, chain.Status) {
	return nil, chain.Status{Kind: "ok"}
}

func (f *fakeBindingAdapter) MemoBindings(context.Context, uint64) ([]chain.MemoBinding, error) {
	return f.memos, nil
}

func (f *fakeBindingAdapter) CrossCheckBinding(_ context.Context, mb chain.MemoBinding) (chain.Verdict, error) {
	status, ok := f.verdicts[mb.TxID]
	if !ok {
		status = "confirmed"
	}
	return chain.Verdict{Status: status}, nil
}

func hearthFor(t *testing.T, seed string) string {
	t.Helper()
	_, pub, err := crypto.GenerateKeyPair([]byte(seed))
	require.NoError(t, err)
	h, err := hearthaddr.New('H', pub)
	require.NoError(t, err)
	return h
}

func TestPollHarvestsMemoBindings(t *testing.T) {
	dataDir := t.TempDir()
	first := hearthFor(t, "harvest one")
	second := hearthFor(t, "harvest two")
	unconfirmed := hearthFor(t, "harvest unconfirmed")
	adapter := &fakeBindingAdapter{
		tip: 508500000,
		memos: []chain.MemoBinding{
			{Source: "alicewyl1235", Hearth: first, TxID: "trx-one", Height: 508400100},
			{Source: "alicewyl1235", Hearth: second, TxID: "trx-two", Height: 508400200},
			{Source: "bobbuilder12", Hearth: unconfirmed, TxID: "trx-bad", Height: 508400300},
			{Source: "carolina1234", Hearth: first, TxID: "trx-late", Height: 508600000}, // beyond tip
		},
		verdicts: map[string]string{"trx-bad": "mismatch"},
	}
	reg, err := bindings.Load(filepath.Join(dataDir, "bindings.jsonl"), 'H')
	require.NoError(t, err)
	w := &watcher.Watcher{Adapter: adapter, ChainCfg: testChainConfig(), DataDir: dataDir, Registry: reg}

	require.NoError(t, w.Poll(t.Context()))

	hearth, ok := reg.HearthFor("alicewyl1235")
	require.True(t, ok)
	assert.Equal(t, second, hearth, "ascending order applies, the latest memo wins")
	_, ok = reg.HearthFor("bobbuilder12")
	assert.False(t, ok, "a cross-check mismatch never enters the registry")
	_, ok = reg.HearthFor("carolina1234")
	assert.False(t, ok, "memos beyond the finalized tip wait for the next poll")

	// Re-polling must not grow the artifact.
	require.NoError(t, w.Poll(t.Context()))
	records, err := store.ReadJSONL[bindings.Record](filepath.Join(dataDir, "bindings.jsonl"))
	require.NoError(t, err)
	assert.Len(t, records, 2)
	assert.Equal(t, "eos-memo-v1", records[0].Format)
	assert.Equal(t, "trx-one", records[0].TxID)
}
