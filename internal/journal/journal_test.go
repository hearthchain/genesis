package journal_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/journal"
)

const wavesCSV = "../../data/journal/waves.csv"

func date(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	require.NoError(t, err)
	return d
}

func TestMaxSincePinsSpecValues(t *testing.T) {
	j, err := journal.Load(filepath.Clean(wavesCSV))
	require.NoError(t, err)

	// The spec's worked example: held since March 2022 -> peak week 2022-04-03,
	// weekly average $49.7131745699 -> 49_713_174 micro-USD (truncated).
	price, week, err := j.MaxSince(date(t, "2022-03-14"))
	require.NoError(t, err)
	assert.Equal(t, uint64(49_713_174), price)
	assert.Equal(t, "2022-04-03", week)

	// Bought early 2024 -> peak week 2024-03-17, $3.99679315047143.
	price, week, err = j.MaxSince(date(t, "2024-01-01"))
	require.NoError(t, err)
	assert.Equal(t, uint64(3_996_793), price)
	assert.Equal(t, "2024-03-17", week)
}

func TestMaxSinceBeforeFirstWeekIsGlobalMaximum(t *testing.T) {
	j, err := journal.Load(filepath.Clean(wavesCSV))
	require.NoError(t, err)

	price, week, err := j.MaxSince(date(t, "2010-01-01"))
	require.NoError(t, err)
	assert.Equal(t, "2022-04-03", week, "WAVES all-time weekly-average peak")
	assert.Equal(t, uint64(49_713_174), price)
}

func TestMaxSinceAfterLastWeekFails(t *testing.T) {
	j, err := journal.Load(filepath.Clean(wavesCSV))
	require.NoError(t, err)

	_, _, err = j.MaxSince(date(t, "2999-01-01"))
	assert.Error(t, err, "no journal weeks at or after the date")
}

func TestLoadRejectsHeaderlessCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-header.csv")
	require.NoError(t, os.WriteFile(path, []byte("2016-06-05,0.9\n2016-06-12,1.1\n"), 0o600))

	_, err := journal.Load(path)
	assert.ErrorContains(t, err, "header", "a data row in the header position means silent data loss")
}
