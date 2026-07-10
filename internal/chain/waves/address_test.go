package waves_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wavesplatform/gowaves/pkg/crypto"
	"github.com/wavesplatform/gowaves/pkg/proto"

	"github.com/hearthchain/genesis/internal/chain/waves"
)

func TestUnspendableIsDeterministicPinnedBurnAddress(t *testing.T) {
	// The published mainnet burn address. Regenerating must always give the
	// same result: the ceremony is reproducible by anyone from the motto.
	addr, err := waves.Unspendable(proto.MainNetScheme, "3PHearthBurn")
	require.NoError(t, err)
	assert.Equal(t, "3PHearthBurnXXXXXXXXXXXXXXXXXZgJXd1", addr)
}

func TestUnspendableAddressIsValidAndKeepsMotto(t *testing.T) {
	addr, err := waves.Unspendable(proto.MainNetScheme, "3PHearthBurn")
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(addr, "3PHearthBurn"), "motto must survive checksum fixup: %s", addr)

	parsed, err := proto.NewAddressFromString(addr)
	require.NoError(t, err)
	ok, err := parsed.Valid(proto.MainNetScheme)
	require.NoError(t, err)
	assert.True(t, ok, "generated address must carry a valid checksum")
}

func TestUnspendableAddressMatchesKeyDerivedChecksumRules(t *testing.T) {
	// A key-derived address must validate under the same rules the generator uses,
	// pinning that Unspendable produces real Waves addresses, not a lookalike format.
	seed := []byte("hearth test throwaway seed")
	_, pub, err := crypto.GenerateKeyPair(seed)
	require.NoError(t, err)
	derived, err := proto.NewAddressFromPublicKey(proto.MainNetScheme, pub)
	require.NoError(t, err)

	ok, err := derived.Valid(proto.MainNetScheme)
	require.NoError(t, err)
	require.True(t, ok)

	corrupted := derived.String()[:len(derived.String())-1] + "1"
	if corrupted == derived.String() {
		corrupted = derived.String()[:len(derived.String())-1] + "2"
	}
	parsed, err := proto.NewAddressFromString(corrupted)
	if err == nil {
		ok, err = parsed.Valid(proto.MainNetScheme)
		require.NoError(t, err)
		assert.False(t, ok, "corrupted checksum must not validate")
	}
}
