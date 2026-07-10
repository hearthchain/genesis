package hearthaddr_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wavesplatform/gowaves/pkg/crypto"

	"github.com/hearthchain/genesis/internal/hearthaddr"
)

func TestValidateAcceptsKeyDerivedHearthAddress(t *testing.T) {
	_, pub, err := crypto.GenerateKeyPair([]byte("hearth cabinet throwaway seed"))
	require.NoError(t, err)

	addr, err := hearthaddr.New('H', pub)
	require.NoError(t, err)

	assert.NoError(t, hearthaddr.Validate(addr, 'H'))
}

func TestValidateRejectsForeignSchemeAndCorruption(t *testing.T) {
	_, pub, err := crypto.GenerateKeyPair([]byte("hearth cabinet throwaway seed"))
	require.NoError(t, err)
	addr, err := hearthaddr.New('H', pub)
	require.NoError(t, err)

	assert.Error(t, hearthaddr.Validate(addr, 'W'), "hearth address must not validate as a Waves one")

	corrupted := addr[:len(addr)-1] + "1"
	if corrupted == addr {
		corrupted = addr[:len(addr)-1] + "2"
	}
	assert.Error(t, hearthaddr.Validate(corrupted, 'H'), "corrupted checksum must fail")

	assert.Error(t, hearthaddr.Validate("", 'H'))
}
