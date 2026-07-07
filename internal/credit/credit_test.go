package credit_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/credit"
	"github.com/hearthchain/burning-page/internal/journal"
	"github.com/hearthchain/burning-page/internal/layers"
)

func loadJournal(t *testing.T) *journal.Journal {
	t.Helper()
	j, err := journal.Load("../../data/journal/waves.csv")
	require.NoError(t, err)
	return j
}

func TestComputePinsTheSpecExample(t *testing.T) {
	// 1000 WAVES held since March 2022: max weekly average $49.713174 for the
	// week of 2022-04-03 -> 49713.174 HRTH.
	j := loadJournal(t)
	consumed := []layers.Layer{
		{Amount: 100_000_000_000, Since: time.Date(2022, 3, 14, 0, 0, 0, 0, time.UTC)},
	}

	total, perLayer, err := credit.Compute(consumed, j)
	require.NoError(t, err)

	assert.Equal(t, big.NewInt(49_713_174_000), total, "micro-HRTH")
	require.Len(t, perLayer, 1)
	assert.Equal(t, "2022-04-03", perLayer[0].WeekEnd)
	assert.Equal(t, uint64(49_713_174), perLayer[0].PriceMicroUSD)
	assert.Equal(t, "49713174000", perLayer[0].CreditMicro)
}

func TestComputeSumsLayersWithTheirOwnDates(t *testing.T) {
	// 1000 WAVES from 2022 plus 1000 WAVES from early 2024: the second layer
	// gets the 2024 peak of $3.996793, roughly 12x less.
	j := loadJournal(t)
	consumed := []layers.Layer{
		{Amount: 100_000_000_000, Since: time.Date(2022, 3, 14, 0, 0, 0, 0, time.UTC)},
		{Amount: 100_000_000_000, Since: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	total, perLayer, err := credit.Compute(consumed, j)
	require.NoError(t, err)

	require.Len(t, perLayer, 2)
	assert.Equal(t, "2024-03-17", perLayer[1].WeekEnd)
	want := new(big.Int).Add(big.NewInt(49_713_174_000), big.NewInt(3_996_793_000))
	assert.Equal(t, want, total)
}

func TestComputeTruncatesSubMicroRemainders(t *testing.T) {
	// 1 wavelet at $49.713174: 1 * 49713174 / 1e8 truncates to 0 micro-HRTH.
	j := loadJournal(t)
	consumed := []layers.Layer{
		{Amount: 1, Since: time.Date(2022, 3, 14, 0, 0, 0, 0, time.UTC)},
	}
	total, _, err := credit.Compute(consumed, j)
	require.NoError(t, err)
	assert.Equal(t, big.NewInt(0), total)
}

func TestComputeFailsWhenJournalHasNoWeeks(t *testing.T) {
	j := loadJournal(t)
	consumed := []layers.Layer{
		{Amount: 100, Since: time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	_, _, err := credit.Compute(consumed, j)
	assert.Error(t, err)
}
