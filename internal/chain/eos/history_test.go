package eos_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/chain/eos"
)

// fakeChainAPI serves scripted balances (one per CombinedBalance call) and a
// fixed account creation time.
type fakeChainAPI struct {
	balances []uint64
	call     int
	created  time.Time
}

func (f *fakeChainAPI) CombinedBalance(context.Context, string) (uint64, error) {
	b := f.balances[f.call]
	if f.call < len(f.balances)-1 {
		f.call++
	}
	return b, nil
}

func (f *fakeChainAPI) AccountCreated(context.Context, string) (time.Time, error) {
	return f.created, nil
}

func (f *fakeChainAPI) LastIrreversibleBlock(context.Context) (uint64, error) {
	return 508500000, nil
}

type fakeHistoryAPI struct {
	rows []json.RawMessage
	err  error
}

func (f *fakeHistoryAPI) TransferActions(context.Context, string, int) ([]json.RawMessage, error) {
	return f.rows, f.err
}

func (f *fakeHistoryAPI) TransfersTo(context.Context, string, int) ([]json.RawMessage, error) {
	return f.rows, f.err
}

var oldAccount = time.Date(2018, 6, 9, 0, 0, 0, 0, time.UTC)

func TestBuildHistorySynthesizesOpeningLayer(t *testing.T) {
	// Balance 100.0000 A; indexed history nets +1.5000: the remaining
	// 98.5000 predates the public index and opens at the boundary.
	api := &fakeChainAPI{balances: []uint64{1_000_000}, created: oldAccount}
	hist := &fakeHistoryAPI{rows: []json.RawMessage{
		action(1001, 508400100, "in0001", "core.vaulta", "someoneelse1", me, "2.0000 A", ""),
		action(1002, 508400200, "out001", "eosio.token", me, "someoneelse1", "0.5000 EOS", ""),
	}}

	h, err := eos.BuildHistory(t.Context(), api, hist, me, 508500000)
	require.NoError(t, err)
	assert.Equal(t, chain.StatusOK, h.Status)
	assert.Equal(t, me, h.Address)
	assert.Equal(t, uint64(508500000), h.ReferenceHeight)
	assert.Equal(t, uint64(1_000_000), h.NodeBalance)
	assert.Equal(t, int64(15_000), h.Recomputed)
	assert.Equal(t, uint64(985_000), h.OpeningBaseUnits)
	assert.Equal(t, time.Date(2023, 3, 18, 0, 0, 0, 0, time.UTC), h.OpeningAt)
	assert.Len(t, h.Txs, 2)
}

func TestBuildHistoryYoungAccountNeedsNoOpening(t *testing.T) {
	api := &fakeChainAPI{balances: []uint64{20_000}, created: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)}
	hist := &fakeHistoryAPI{rows: []json.RawMessage{
		action(1101, 508400100, "in0002", "core.vaulta", "someoneelse1", me, "2.0000 A", ""),
	}}

	h, err := eos.BuildHistory(t.Context(), api, hist, me, 508500000)
	require.NoError(t, err)
	assert.Equal(t, chain.StatusOK, h.Status)
	assert.Zero(t, h.OpeningBaseUnits, "an account born after the index floor has complete history")
}

func TestBuildHistoryBlocksYoungAccountWithRemainder(t *testing.T) {
	// Created after the index floor, yet the balance exceeds the replayed
	// sum: the index missed something. Never guess.
	api := &fakeChainAPI{balances: []uint64{100_000}, created: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)}
	hist := &fakeHistoryAPI{rows: []json.RawMessage{
		action(1201, 508400100, "in0003", "core.vaulta", "someoneelse1", me, "2.0000 A", ""),
	}}

	h, err := eos.BuildHistory(t.Context(), api, hist, me, 508500000)
	require.NoError(t, err)
	assert.Equal(t, chain.StatusUnsupported, h.Status)
	assert.Contains(t, h.Reason, "created after the index floor")
}

func TestBuildHistoryBlocksWhenSumExceedsBalance(t *testing.T) {
	api := &fakeChainAPI{balances: []uint64{10_000}, created: oldAccount}
	hist := &fakeHistoryAPI{rows: []json.RawMessage{
		action(1301, 508400100, "in0004", "core.vaulta", "someoneelse1", me, "2.0000 A", ""),
	}}

	h, err := eos.BuildHistory(t.Context(), api, hist, me, 508500000)
	require.NoError(t, err)
	assert.Equal(t, chain.StatusUnsupported, h.Status)
	assert.Contains(t, h.Reason, "exceeds")
}

func TestBuildHistoryRetriesOnceWhenBalanceMoves(t *testing.T) {
	// First attempt sees the balance move mid-fetch; the retry sees it settle.
	api := &fakeChainAPI{balances: []uint64{10_000, 20_000, 20_000, 20_000}, created: oldAccount}
	hist := &fakeHistoryAPI{rows: []json.RawMessage{
		action(1401, 508400100, "in0005", "core.vaulta", "someoneelse1", me, "2.0000 A", ""),
	}}

	h, err := eos.BuildHistory(t.Context(), api, hist, me, 508500000)
	require.NoError(t, err)
	assert.Equal(t, chain.StatusOK, h.Status)
	assert.Equal(t, uint64(20_000), h.NodeBalance)
}

func TestBuildHistoryBlocksWhenBalanceKeepsMoving(t *testing.T) {
	api := &fakeChainAPI{balances: []uint64{10_000, 20_000, 30_000, 40_000, 50_000}, created: oldAccount}
	hist := &fakeHistoryAPI{rows: nil}

	h, err := eos.BuildHistory(t.Context(), api, hist, me, 508500000)
	require.NoError(t, err)
	assert.Equal(t, chain.StatusUnsupported, h.Status)
	assert.Contains(t, h.Reason, "balance moved")
}

func TestBuildHistoryBlocksOversizedHistories(t *testing.T) {
	api := &fakeChainAPI{balances: []uint64{10_000}, created: oldAccount}
	hist := &fakeHistoryAPI{err: eos.ErrHistoryTooLarge}

	h, err := eos.BuildHistory(t.Context(), api, hist, me, 508500000)
	require.NoError(t, err)
	assert.Equal(t, chain.StatusUnsupported, h.Status)
	assert.Contains(t, h.Reason, "cap")
}
