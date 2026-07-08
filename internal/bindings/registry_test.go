package bindings_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wavesplatform/gowaves/pkg/crypto"
	"github.com/wavesplatform/gowaves/pkg/proto"

	"github.com/hearthchain/burning-page/internal/binding"
	"github.com/hearthchain/burning-page/internal/bindings"
	"github.com/hearthchain/burning-page/internal/hearthaddr"
)

func signedRecord(t *testing.T, seed, hearthSeed string) bindings.Record {
	t.Helper()
	sec, pub, err := crypto.GenerateKeyPair([]byte(seed))
	require.NoError(t, err)
	source, err := proto.NewAddressFromPublicKey(proto.MainNetScheme, pub)
	require.NoError(t, err)
	_, hearthPub, err := crypto.GenerateKeyPair([]byte(hearthSeed))
	require.NoError(t, err)
	hearth, err := hearthaddr.New('H', hearthPub)
	require.NoError(t, err)
	sig, err := crypto.Sign(sec, binding.Message(source.String(), hearth))
	require.NoError(t, err)
	return bindings.Record{
		Source:    source.String(),
		Chain:     "waves",
		Hearth:    hearth,
		PublicKey: pub.String(),
		Signature: sig.String(),
	}
}

func TestAddVerifiesPersistsAndResolves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bindings.jsonl")
	reg, err := bindings.Load(path, 'H')
	require.NoError(t, err)

	rec := signedRecord(t, "seed one", "hearth one")
	require.NoError(t, reg.Add(rec))

	hearth, ok := reg.HearthFor(rec.Source)
	require.True(t, ok)
	assert.Equal(t, rec.Hearth, hearth)

	// A fresh registry over the same file sees the persisted binding.
	reg2, err := bindings.Load(path, 'H')
	require.NoError(t, err)
	hearth2, ok := reg2.HearthFor(rec.Source)
	require.True(t, ok)
	assert.Equal(t, rec.Hearth, hearth2)

	assert.Equal(t, []string{rec.Source}, reg2.SourcesFor(rec.Hearth))
}

func TestAddRejectsInvalidSignature(t *testing.T) {
	reg, err := bindings.Load(filepath.Join(t.TempDir(), "b.jsonl"), 'H')
	require.NoError(t, err)

	rec := signedRecord(t, "seed one", "hearth one")
	other := signedRecord(t, "seed two", "hearth two")
	rec.Signature = other.Signature // signature over a different message

	assert.Error(t, reg.Add(rec))
	_, ok := reg.HearthFor(rec.Source)
	assert.False(t, ok)
}

func TestLatestBindingWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.jsonl")
	reg, err := bindings.Load(path, 'H')
	require.NoError(t, err)

	first := signedRecord(t, "seed one", "hearth one")
	require.NoError(t, reg.Add(first))

	// Same source key, new hearth destination.
	second := signedRecord(t, "seed one", "hearth replacement")
	require.NoError(t, reg.Add(second))
	require.Equal(t, first.Source, second.Source)

	hearth, ok := reg.HearthFor(first.Source)
	require.True(t, ok)
	assert.Equal(t, second.Hearth, hearth)
	assert.Empty(t, reg.SourcesFor(first.Hearth), "the replaced hearth loses the source")
}

func TestCountReportsBoundSources(t *testing.T) {
	reg, err := bindings.Load(filepath.Join(t.TempDir(), "b.jsonl"), 'H')
	require.NoError(t, err)
	assert.Equal(t, 0, reg.Count())

	require.NoError(t, reg.Add(signedRecord(t, "seed one", "hearth one")))
	assert.Equal(t, 1, reg.Count())

	// Rebinding the same source replaces, not adds.
	require.NoError(t, reg.Add(signedRecord(t, "seed one", "hearth replacement")))
	assert.Equal(t, 1, reg.Count())

	require.NoError(t, reg.Add(signedRecord(t, "seed two", "hearth two")))
	assert.Equal(t, 2, reg.Count())
}

func keeperSignedRecord(t *testing.T, seed, hearthSeed string) bindings.Record {
	t.Helper()
	rec := signedRecord(t, seed, hearthSeed)
	sec, _, err := crypto.GenerateKeyPair([]byte(seed))
	require.NoError(t, err)
	sig, err := crypto.Sign(sec, binding.KeeperV1Envelope(binding.Message(rec.Source, rec.Hearth)))
	require.NoError(t, err)
	rec.Signature = sig.String()
	rec.Format = "keeper-v1"
	return rec
}

func TestAddDispatchesByFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.jsonl")
	reg, err := bindings.Load(path, 'H')
	require.NoError(t, err)

	keeper := keeperSignedRecord(t, "keeper seed", "keeper hearth")
	require.NoError(t, reg.Add(keeper))
	hearth, ok := reg.HearthFor(keeper.Source)
	require.True(t, ok)
	assert.Equal(t, keeper.Hearth, hearth)

	// The same signature under the raw format must fail: formats don't cross.
	crossed := keeper
	crossed.Format = ""
	assert.ErrorIs(t, reg.Add(crossed), binding.ErrBadSignature)

	unknown := signedRecord(t, "unknown seed", "unknown hearth")
	unknown.Format = "keeper-v2"
	assert.ErrorContains(t, reg.Add(unknown), "format")
}
