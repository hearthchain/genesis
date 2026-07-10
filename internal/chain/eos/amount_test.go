package eos_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/chain/eos"
)

func TestParseQuantityAcceptsBothNativeTokens(t *testing.T) {
	tests := []struct {
		name     string
		quantity string
		want     uint64
	}{
		{"whole A", "1.0000 A", 10_000},
		{"dust A", "0.0001 A", 1},
		{"legacy EOS", "12.3456 EOS", 123_456},
		{"large balance", "965244285.0000 EOS", 9_652_442_850_000},
		{"zero", "0.0000 A", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := eos.ParseQuantity(tc.quantity)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseQuantityRejectsForeignShapes(t *testing.T) {
	tests := []struct {
		name     string
		quantity string
	}{
		{"wrong precision", "1.00 A"},
		{"too many decimals", "1.00000 A"},
		{"no decimals", "1 A"},
		{"foreign symbol", "1.0000 WAX"},
		{"missing symbol", "1.0000"},
		{"junk", "lots of tokens"},
		{"negative", "-1.0000 A"},
		{"empty", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := eos.ParseQuantity(tc.quantity)
			assert.Error(t, err)
		})
	}
}
