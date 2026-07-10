package evidence_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hearthchain/burning-page/internal/credit"
	"github.com/hearthchain/burning-page/internal/evidence"
)

func sample() evidence.Bundle {
	return evidence.Bundle{
		TxID:            "B1",
		Chain:           "waves",
		Source:          "3PSource",
		Hearth:          "HDest",
		AmountBaseUnits: 100_000_000_000,
		Height:          4000010,
		Layers: []credit.LayerCredit{{
			AmountBaseUnits: 100_000_000_000,
			Since:           "2022-03-14T00:00:00Z",
			WeekEnd:         "2022-04-03",
			PriceMicroUSD:   49_713_174,
			CreditMicro:     "49713174000",
		}},
		CreditMicro:     "49713174000",
		ReferenceHeight: 4000100,
		TransfersSha256: "deadbeef",
	}
}

func TestMarshalIsDeterministic(t *testing.T) {
	a, err := sample().Marshal()
	require.NoError(t, err)
	b, err := sample().Marshal()
	require.NoError(t, err)
	assert.Equal(t, a, b, "byte-identical on repeated runs")
	assert.True(t, json.Valid(a))

	sumA, err := sample().Sha256()
	require.NoError(t, err)
	sumB, err := sample().Sha256()
	require.NoError(t, err)
	assert.Equal(t, sumA, sumB)
	assert.Len(t, sumA, 64)
}

func TestMarshalRoundTrips(t *testing.T) {
	raw, err := sample().Marshal()
	require.NoError(t, err)
	var back evidence.Bundle
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, sample(), back)
}
