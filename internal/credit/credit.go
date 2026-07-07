// Package credit implements the credit formula: each consumed layer is priced
// at the maximum weekly-average price since the layer's date, and one credit
// unit equals one HRTH token. All arithmetic is integer (wavelets, micro-USD,
// micro-HRTH big.Int); sub-micro remainders truncate, and the published rule
// is the truncation.
package credit

import (
	"fmt"
	"math/big"

	"github.com/hearthchain/burning-page/internal/journal"
	"github.com/hearthchain/burning-page/internal/layers"
)

// wavesInWavelets converts the wavelets x micro-USD product to micro-HRTH:
// credit_microHRTH = wavelets * price_microUSD / 1e8.
const wavesInWavelets = 100_000_000

// LayerCredit is the per-layer breakdown of a credit, evidence-bundle ready.
type LayerCredit struct {
	AmountWavelets uint64 `json:"amountWavelets"`
	Since          string `json:"since"`
	WeekEnd        string `json:"weekEnd"`
	PriceMicroUSD  uint64 `json:"priceMicroUsd"`
	CreditMicro    string `json:"creditMicro"`
}

// Compute prices the consumed layers against the journal and returns the
// total credit in micro-HRTH plus the per-layer breakdown.
func Compute(consumed []layers.Layer, j *journal.Journal) (*big.Int, []LayerCredit, error) {
	total := new(big.Int)
	perLayer := make([]LayerCredit, 0, len(consumed))
	for _, l := range consumed {
		price, weekEnd, err := j.MaxSince(l.Since)
		if err != nil {
			return nil, nil, fmt.Errorf("credit: layer since %s: %w", l.Since, err)
		}
		c := new(big.Int).SetUint64(l.Amount)
		c.Mul(c, new(big.Int).SetUint64(price))
		c.Quo(c, big.NewInt(wavesInWavelets))
		total.Add(total, c)
		perLayer = append(perLayer, LayerCredit{
			AmountWavelets: l.Amount,
			Since:          l.Since.Format("2006-01-02T15:04:05Z07:00"),
			WeekEnd:        weekEnd,
			PriceMicroUSD:  price,
			CreditMicro:    c.String(),
		})
	}
	return total, perLayer, nil
}
