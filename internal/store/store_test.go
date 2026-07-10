package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/genesis/internal/store"
)

type rec struct {
	ID string `json:"id"`
	N  int    `json:"n"`
}

func TestAppendAndReadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.jsonl")

	require.NoError(t, store.AppendJSONL(path, rec{ID: "a", N: 1}))
	require.NoError(t, store.AppendJSONL(path, rec{ID: "b", N: 2}))

	got, err := store.ReadJSONL[rec](path)
	require.NoError(t, err)
	assert.Equal(t, []rec{{ID: "a", N: 1}, {ID: "b", N: 2}}, got)
}

func TestReadMissingFileIsEmptyNotError(t *testing.T) {
	got, err := store.ReadJSONL[rec](filepath.Join(t.TempDir(), "absent.jsonl"))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAppendCreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "nested", "x.jsonl")
	require.NoError(t, store.AppendJSONL(path, rec{ID: "a"}))
	_, err := os.Stat(path)
	assert.NoError(t, err)
}

func TestSha256File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0o600))

	sum, err := store.Sha256File(path)
	require.NoError(t, err)
	assert.Equal(t, "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03", sum)
}
