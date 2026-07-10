// Package layers reconstructs the holding-layer profile of an address from
// its balance-delta history: the running-minimum accounting the spec defines.
// A deposit opens a layer dated at its transaction; an ordinary spend trims
// layers newest-first, so old coins keep their date; a burn consumes layers
// oldest-first and the consumption is recorded per burn transaction, because
// that is what the credit formula prices.
package layers

import (
	"fmt"
	"sort"
	"time"

	"github.com/hearthchain/burning-page/internal/chain"
)

// Layer is a quantity of coins continuously present on the address since a date.
type Layer struct {
	Amount uint64    `json:"amountBaseUnits"`
	Since  time.Time `json:"since"`
}

// Build replays the deltas in height order and returns the final layer
// profile plus, for every burn transaction (txID -> burned amount), the
// oldest-first layers that burn consumed.
func Build(deltas []chain.Delta, burnAmounts map[string]uint64) ([]Layer, map[string][]Layer, error) {
	ordered := make([]chain.Delta, len(deltas))
	copy(ordered, deltas)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Height < ordered[j].Height })

	var profile []Layer
	consumed := make(map[string][]Layer)
	for _, d := range ordered {
		var err error
		profile, err = applyDelta(profile, consumed, d, burnAmounts[d.TxID])
		if err != nil {
			return nil, nil, err
		}
	}
	return profile, consumed, nil
}

func applyDelta(profile []Layer, consumed map[string][]Layer, d chain.Delta, burned uint64) ([]Layer, error) {
	if d.Amount >= 0 {
		if d.Amount > 0 {
			profile = append(profile, Layer{Amount: uint64(d.Amount), Since: d.Timestamp})
		}
		return profile, nil
	}
	spend := uint64(-d.Amount)
	if burned > 0 {
		if burned > spend {
			return nil, fmt.Errorf("layers: tx %s burns %d but only %d left the address", d.TxID, burned, spend)
		}
		var taken []Layer
		var err error
		profile, taken, err = takeOldest(profile, burned, d.TxID)
		if err != nil {
			return nil, err
		}
		consumed[d.TxID] = taken
		spend -= burned
	}
	return trimNewest(profile, spend, d.TxID)
}

// takeOldest consumes amount from the oldest layers and returns what was taken.
func takeOldest(profile []Layer, amount uint64, txID string) ([]Layer, []Layer, error) {
	var taken []Layer
	for amount > 0 {
		if len(profile) == 0 {
			return nil, nil, fmt.Errorf("layers: tx %s overdraws the layer profile", txID)
		}
		oldest := &profile[0]
		take := min(oldest.Amount, amount)
		taken = append(taken, Layer{Amount: take, Since: oldest.Since})
		oldest.Amount -= take
		amount -= take
		if oldest.Amount == 0 {
			profile = profile[1:]
		}
	}
	return profile, taken, nil
}

// trimNewest consumes amount from the newest layers (ordinary spend).
func trimNewest(profile []Layer, amount uint64, txID string) ([]Layer, error) {
	for amount > 0 {
		if len(profile) == 0 {
			return nil, fmt.Errorf("layers: tx %s drives the balance negative", txID)
		}
		newest := &profile[len(profile)-1]
		take := min(newest.Amount, amount)
		newest.Amount -= take
		amount -= take
		if newest.Amount == 0 {
			profile = profile[:len(profile)-1]
		}
	}
	return profile, nil
}
