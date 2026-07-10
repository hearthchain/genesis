package waves_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/chain"
	"github.com/hearthchain/burning-page/internal/chain/waves"
)

// fakePrimary serves canned data for the adapter's primary-node surface.
type fakePrimary struct {
	height   uint64
	txs      map[string][]json.RawMessage
	balances map[string]uint64
}

func (f *fakePrimary) AllTransactions(_ context.Context, addr string) ([]json.RawMessage, error) {
	return f.txs[addr], nil
}

func (f *fakePrimary) Height(context.Context) (uint64, error) { return f.height, nil }

func (f *fakePrimary) BalanceAfterConfirmations(_ context.Context, addr string, _ uint64) (uint64, error) {
	return f.balances[addr], nil
}

func transferTx(id, sender, recipient string, amount, fee int64, height uint64) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"type":4,"id":%q,"sender":%q,"recipient":%q,"amount":%d,"fee":%d,"timestamp":1750000000000,"height":%d,"applicationStatus":"succeeded"}`,
		id, sender, recipient, amount, fee, height))
}

func TestAdapterReportsNameAndHeight(t *testing.T) {
	a := &waves.Adapter{Primary: &fakePrimary{height: 4200000}, BurnAddress: burnAddr}

	var _ chain.Adapter = a
	assert.Equal(t, "waves", a.Name())
	h, err := a.Height(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint64(4200000), h)
}

func TestAdapterValidatesMainnetAddresses(t *testing.T) {
	a := &waves.Adapter{}

	assert.NoError(t, a.ValidateAddress(burnAddr))
	assert.Error(t, a.ValidateAddress("not-an-address"))
	assert.Error(t, a.ValidateAddress(""))
}

func TestAdapterBurnCandidatesMatchDetectBurns(t *testing.T) {
	window := chain.Window{Start: 4000000, End: 4001000}
	txs := burnFixtureTxs(t)
	a := &waves.Adapter{
		Primary:     &fakePrimary{txs: map[string][]json.RawMessage{burnAddr: txs}},
		BurnAddress: burnAddr,
	}

	got, err := a.BurnCandidates(t.Context(), window)
	require.NoError(t, err)
	want, err := waves.DetectBurns(txs, burnAddr, window)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestAdapterHistoryVerifiesBalanceInvariant(t *testing.T) {
	const source = "3PSenderAlice1111111111111111111111"
	in := transferTx("In1", "3PSenderCarol333333333333333333333", source, 500000000, 100000, 4000200)
	out := transferTx("Out2", source, burnAddr, 100000000, 100000, 4000300)
	a := &waves.Adapter{
		Primary: &fakePrimary{
			txs:      map[string][]json.RawMessage{source: {out, in}}, // deliberately unsorted
			balances: map[string]uint64{source: 399900000},            // 500000000 - 100000000 - fee
		},
		BurnAddress: burnAddr,
	}

	h, err := a.History(t.Context(), source, 4000500, 4000600)
	require.NoError(t, err)
	assert.Equal(t, "ok", h.Status)
	assert.Equal(t, source, h.Address)
	assert.Equal(t, uint64(4000500), h.ReferenceHeight)
	assert.Equal(t, uint64(399900000), h.NodeBalance)
	assert.Equal(t, int64(399900000), h.Recomputed)
	assert.Zero(t, h.OpeningBaseUnits, "waves history is complete; no synthetic opening layer")
	require.Len(t, h.Txs, 2)
	var first struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(h.Txs[0], &first))
	assert.Equal(t, "In1", first.ID, "history rows are sorted by ascending height")
}

func TestAdapterHistoryBlocksOnBalanceMismatch(t *testing.T) {
	const source = "3PSenderAlice1111111111111111111111"
	in := transferTx("In1", "3PSenderCarol333333333333333333333", source, 500000000, 100000, 4000200)
	a := &waves.Adapter{
		Primary: &fakePrimary{
			txs:      map[string][]json.RawMessage{source: {in}},
			balances: map[string]uint64{source: 123}, // node disagrees with the recomputed sum
		},
		BurnAddress: burnAddr,
	}

	h, err := a.History(t.Context(), source, 4000500, 4000600)
	require.NoError(t, err)
	assert.Equal(t, "unsupported", h.Status)
	assert.Contains(t, h.Reason, "node balance")
}

func TestAdapterHistoryBlocksOnUnsupportedTxType(t *testing.T) {
	const source = "3PSenderAlice1111111111111111111111"
	lease := json.RawMessage(`{"type":8,"id":"Lease1","sender":"` + source + `","amount":1,"height":4000100}`)
	a := &waves.Adapter{
		Primary:     &fakePrimary{txs: map[string][]json.RawMessage{source: {lease}}},
		BurnAddress: burnAddr,
	}

	h, err := a.History(t.Context(), source, 4000500, 4000600)
	require.NoError(t, err)
	assert.Equal(t, "unsupported", h.Status)
	assert.Contains(t, h.Reason, "type 8")
}

func TestAdapterCrossCheckDelegatesToSecondary(t *testing.T) {
	burn, raw := fixtureBurn(t)
	a := &waves.Adapter{Secondary: secondaryNode(t, burn.Height+200, raw)}

	verdict, err := a.CrossCheck(t.Context(), burn, 100)
	require.NoError(t, err)
	assert.Equal(t, "confirmed", verdict.Status)
}
