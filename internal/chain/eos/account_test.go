package eos_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hearthchain/genesis/internal/chain/eos"
)

func TestValidateAccountAcceptsAntelopeNames(t *testing.T) {
	for _, name := range []string{"eosio.null", "core.vaulta", "a", "youraccount1", "num1234.five"} {
		assert.NoError(t, eos.ValidateAccount(name), name)
	}
}

func TestValidateAccountRejectsForeignShapes(t *testing.T) {
	tests := []struct{ name, account string }{
		{"empty", ""},
		{"too long", "thirteenchars"},
		{"uppercase", "MyAccount"},
		{"bad digit", "account6"},
		{"leading dot", ".account"},
		{"trailing dot", "account."},
		{"waves address", "3PHearthBurnXXXXXXXXXXXXXXXXXZgJXd1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Error(t, eos.ValidateAccount(tc.account))
		})
	}
}
