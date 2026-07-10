package binding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wavesplatform/gowaves/pkg/crypto"
	"github.com/wavesplatform/gowaves/pkg/proto"

	"github.com/hearthchain/genesis/internal/binding"
	"github.com/hearthchain/genesis/internal/hearthaddr"
)

func TestMessageIsCanonicalV1(t *testing.T) {
	got := binding.Message("3PSource", "HDest")
	assert.Equal(t, []byte("hearth-genesis-binding:v1:3PSource:HDest"), got)
}

func testKeys(t *testing.T) (crypto.SecretKey, crypto.PublicKey, string, string) {
	t.Helper()
	sec, pub, err := crypto.GenerateKeyPair([]byte("binding test throwaway seed"))
	require.NoError(t, err)
	source, err := proto.NewAddressFromPublicKey(proto.MainNetScheme, pub)
	require.NoError(t, err)
	hearth, err := hearthaddr.New('H', pub)
	require.NoError(t, err)
	return sec, pub, source.String(), hearth
}

func TestVerifyAcceptsSignatureBySourceKey(t *testing.T) {
	sec, pub, source, hearth := testKeys(t)
	sig, err := crypto.Sign(sec, binding.Message(source, hearth))
	require.NoError(t, err)

	err = binding.Verify(source, hearth, 'H', pub.String(), sig.String())
	assert.NoError(t, err)
}

func TestVerifyRejectsWrongKeyCorruptSignatureAndBadHearth(t *testing.T) {
	sec, pub, source, hearth := testKeys(t)
	sig, err := crypto.Sign(sec, binding.Message(source, hearth))
	require.NoError(t, err)

	// Signature by a key that does not own the source address.
	otherSec, otherPub, err := crypto.GenerateKeyPair([]byte("a different seed entirely"))
	require.NoError(t, err)
	otherSig, err := crypto.Sign(otherSec, binding.Message(source, hearth))
	require.NoError(t, err)
	assert.ErrorIs(t, binding.Verify(source, hearth, 'H', otherPub.String(), otherSig.String()), binding.ErrSourceMismatch)

	// Valid key, corrupted signature bytes.
	broken := sig
	broken[0] ^= 0xff
	assert.ErrorIs(t, binding.Verify(source, hearth, 'H', pub.String(), broken.String()), binding.ErrBadSignature)

	// Hearth address with a broken checksum.
	badHearth := hearth[:len(hearth)-1] + "1"
	if badHearth == hearth {
		badHearth = hearth[:len(hearth)-1] + "2"
	}
	assert.Error(t, binding.Verify(source, badHearth, 'H', pub.String(), sig.String()))
}

func TestVerifyKeeperV1AcceptsEnvelopeSignature(t *testing.T) {
	sec, pub, source, hearth := testKeys(t)
	sig, err := crypto.Sign(sec, binding.KeeperV1Envelope(binding.Message(source, hearth)))
	require.NoError(t, err)

	assert.NoError(t, binding.VerifyKeeperV1(source, hearth, 'H', pub.String(), sig.String()))
}

func TestRawAndKeeperFormatsDoNotCrossAccept(t *testing.T) {
	sec, pub, source, hearth := testKeys(t)

	rawSig, err := crypto.Sign(sec, binding.Message(source, hearth))
	require.NoError(t, err)
	envSig, err := crypto.Sign(sec, binding.KeeperV1Envelope(binding.Message(source, hearth)))
	require.NoError(t, err)

	assert.ErrorIs(t, binding.VerifyKeeperV1(source, hearth, 'H', pub.String(), rawSig.String()), binding.ErrBadSignature,
		"a raw signature must not pass as a Keeper envelope one")
	assert.ErrorIs(t, binding.Verify(source, hearth, 'H', pub.String(), envSig.String()), binding.ErrBadSignature,
		"a Keeper envelope signature must not pass as a raw one")
}

func TestKeeperV1EnvelopeLayout(t *testing.T) {
	got := binding.KeeperV1Envelope([]byte("abc"))
	assert.Equal(t, []byte{255, 255, 255, 1, 'a', 'b', 'c'}, got)
}
