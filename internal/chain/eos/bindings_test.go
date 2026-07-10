package eos_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wavesplatform/gowaves/pkg/crypto"

	"github.com/hearthchain/genesis/internal/binding"
	"github.com/hearthchain/genesis/internal/chain/eos"
	"github.com/hearthchain/genesis/internal/hearthaddr"
)

func bindingMemo(source, hearth string) string {
	return string(binding.Message(source, hearth))
}

func hearthAddress(t *testing.T, seed string) string {
	t.Helper()
	_, pub, err := crypto.GenerateKeyPair([]byte(seed))
	require.NoError(t, err)
	h, err := hearthaddr.New('H', pub)
	require.NoError(t, err)
	return h
}

func TestExtractBindingsReadsValidMemos(t *testing.T) {
	hearth := hearthAddress(t, "eos binding test")
	later := hearthAddress(t, "eos binding rebind")
	rows := []json.RawMessage{
		// Ascending on-chain order: the second memo supersedes at the registry.
		action(801, 508400100, "bind01", "core.vaulta", me, burnAccount, "0.0001 A", bindingMemo(me, hearth)),
		action(802, 508400200, "bind02", "core.vaulta", me, burnAccount, "0.0001 A", bindingMemo(me, later)),
	}

	got := eos.ExtractBindings(rows, burnAccount, 'H')
	require.Len(t, got, 2)
	assert.Equal(t, me, got[0].Source)
	assert.Equal(t, hearth, got[0].Hearth)
	assert.Equal(t, "bind01", got[0].TxID)
	assert.Equal(t, uint64(508400100), got[0].Height)
	assert.Equal(t, later, got[1].Hearth, "rows come out in ascending chain order")
}

func TestExtractBindingsAcceptsMemoOnTheBurnItself(t *testing.T) {
	hearth := hearthAddress(t, "eos binding test")
	row := action(811, 508400300, "bigburn1", "core.vaulta", me, burnAccount, "500.0000 A", bindingMemo(me, hearth))

	got := eos.ExtractBindings([]json.RawMessage{row}, burnAccount, 'H')
	require.Len(t, got, 1, "any amount carries a binding, including the burn transfer")
	assert.Equal(t, hearth, got[0].Hearth)
}

func TestExtractBindingsIgnoresInvalidMemos(t *testing.T) {
	hearth := hearthAddress(t, "eos binding test")
	rows := []json.RawMessage{
		// Memo names an account other than the sender: proof fails.
		action(821, 508400400, "bad001", "core.vaulta", me, burnAccount, "0.0001 A", bindingMemo("someoneelse1", hearth)),
		// Hearth address fails validation.
		action(822, 508400401, "bad002", "core.vaulta", me, burnAccount, "0.0001 A", bindingMemo(me, "3Hnothearth")),
		// Plain memos and garbage are simply not bindings.
		action(823, 508400402, "bad003", "core.vaulta", me, burnAccount, "0.0001 A", "minepool burn"),
		action(824, 508400403, "bad004", "core.vaulta", me, burnAccount, "0.0001 A", ""),
		// Not addressed to the burn account.
		action(825, 508400404, "bad005", "core.vaulta", me, "someoneelse1", "0.0001 A", bindingMemo(me, hearth)),
		// Undecodable rows cannot carry a provable binding.
		json.RawMessage(`{not json`),
	}

	got := eos.ExtractBindings(rows, burnAccount, 'H')
	assert.Empty(t, got)
}
