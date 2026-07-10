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

// LayerCredit is the per-layer breakdown of a credit, evidence-bundle ready.
type LayerCredit struct {
	AmountBaseUnits uint64 `json:"amountBaseUnits"`
	Since           string `json:"since"`
	WeekEnd         string `json:"weekEnd"`
	PriceMicroUSD   uint64 `json:"priceMicroUsd"`
	CreditMicro     string `json:"creditMicro"`
}

// Compute prices the consumed layers against the journal and returns the
// total credit in micro-HRTH plus the per-layer breakdown. baseUnits is the
// number of base units in one whole coin on the layer's chain:
// credit_microHRTH = amount_baseUnits * price_microUSD / baseUnits.
func Compute(consumed []layers.Layer, j *journal.Journal, baseUnits uint64) (*big.Int, []LayerCredit, error) {
	total := new(big.Int)
	perLayer := make([]LayerCredit, 0, len(consumed))
	for _, l := range consumed {
		price, weekEnd, err := j.MaxSince(l.Since)
		if err != nil {
			return nil, nil, fmt.Errorf("credit: layer since %s: %w", l.Since, err)
		}
		c := new(big.Int).SetUint64(l.Amount)
		c.Mul(c, new(big.Int).SetUint64(price))
		c.Quo(c, new(big.Int).SetUint64(baseUnits))
		total.Add(total, c)
		perLayer = append(perLayer, LayerCredit{
			AmountBaseUnits: l.Amount,
			Since:           l.Since.Format("2006-01-02T15:04:05Z07:00"),
			WeekEnd:         weekEnd,
			PriceMicroUSD:   price,
			CreditMicro:     c.String(),
		})
	}
	return total, perLayer, nil
}
