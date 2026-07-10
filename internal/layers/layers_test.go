package layers_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/layers"
)

var (
	t1 = time.Date(2022, 3, 14, 0, 0, 0, 0, time.UTC)
	t2 = time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	t3 = time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
)

func delta(id string, h uint64, ts time.Time, amount int64) chain.Delta {
	return chain.Delta{TxID: id, Height: h, Timestamp: ts, Amount: amount}
}

func TestBuildTableDriven(t *testing.T) {
	tests := []struct {
		name    string
		deltas  []chain.Delta
		burns   map[string]uint64
		profile []layers.Layer
		wantErr bool
	}{
		{
			name:    "single deposit is one layer",
			deltas:  []chain.Delta{delta("a", 100, t1, 1000)},
			profile: []layers.Layer{{Amount: 1000, Since: t1}},
		},
		{
			name:    "top-up adds a layer and resets nothing",
			deltas:  []chain.Delta{delta("a", 100, t1, 600), delta("b", 200, t2, 400)},
			profile: []layers.Layer{{Amount: 600, Since: t1}, {Amount: 400, Since: t2}},
		},
		{
			name:    "sale trims newest-first, old coins keep their date",
			deltas:  []chain.Delta{delta("a", 100, t1, 600), delta("b", 200, t2, 400), delta("c", 300, t3, -400)},
			profile: []layers.Layer{{Amount: 600, Since: t1}},
		},
		{
			name:    "sale dipping into the old layer trims it too",
			deltas:  []chain.Delta{delta("a", 100, t1, 1000), delta("b", 300, t3, -800)},
			profile: []layers.Layer{{Amount: 200, Since: t1}},
		},
		{
			name:    "redeposit after a dip starts a fresh layer",
			deltas:  []chain.Delta{delta("a", 100, t1, 1000), delta("b", 200, t2, -800), delta("c", 300, t3, 500)},
			profile: []layers.Layer{{Amount: 200, Since: t1}, {Amount: 500, Since: t3}},
		},
		{
			name:    "negative balance is a data error",
			deltas:  []chain.Delta{delta("a", 100, t1, 100), delta("b", 200, t2, -200)},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile, _, err := layers.Build(tc.deltas, tc.burns)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.profile, profile)
		})
	}
}

func TestBurnConsumesOldestLayersFirstAndFeeConsumesNewest(t *testing.T) {
	// Layers 600@t1 and 400@t2; a burn tx destroys 700 and pays 1 fee.
	deltas := []chain.Delta{
		delta("dep1", 100, t1, 600),
		delta("dep2", 200, t2, 400),
		delta("burn", 300, t3, -701),
	}
	profile, consumed, err := layers.Build(deltas, map[string]uint64{"burn": 700})
	require.NoError(t, err)

	require.Contains(t, consumed, "burn")
	assert.Equal(t, []layers.Layer{{Amount: 600, Since: t1}, {Amount: 100, Since: t2}}, consumed["burn"],
		"the spec: partial burns spend the oldest layers")
	assert.Equal(t, []layers.Layer{{Amount: 299, Since: t2}}, profile,
		"the fee is an ordinary spend and trims the newest remainder")
}

func TestBuildSortsByHeight(t *testing.T) {
	deltas := []chain.Delta{
		delta("late", 300, t3, -500),
		delta("early", 100, t1, 1000),
	}
	profile, _, err := layers.Build(deltas, nil)
	require.NoError(t, err)
	assert.Equal(t, []layers.Layer{{Amount: 500, Since: t1}}, profile)
}
