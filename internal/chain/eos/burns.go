package eos

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/hearthchain/burning-page/internal/chain"
)

// Chain is the chain slug used in artifacts, routes and config.
const Chain = "eos"

// DetectBurns filters transfer rows down to the burns inside the window:
// native-token transfers to the burn account with a block height in
// [window.Start, window.End]. Rows of one transaction from one sender sum
// into one burn (a transaction may burn A and legacy EOS together), and
// Burn.Raw keeps the verbatim action rows as a JSON array. Memos are
// tolerated here; binding memos are interpreted separately.
func DetectBurns(rows []json.RawMessage, burnAccount string, window chain.Window) ([]chain.Burn, error) {
	type group struct {
		burn chain.Burn
		raws []json.RawMessage
		seen map[uint64]bool
	}
	groups := map[string]*group{}
	for _, raw := range rows {
		row, units, err := decodeAction(raw)
		if err != nil {
			return nil, fmt.Errorf("eos: burn detection: %w", err)
		}
		if row.Act.Data.To != burnAccount || row.BlockNum < window.Start || row.BlockNum > window.End {
			continue
		}
		key := row.TrxID + "/" + row.Act.Data.From
		g, ok := groups[key]
		if !ok {
			ts, tErr := rowTime(row.Timestamp)
			if tErr != nil {
				return nil, fmt.Errorf("eos: burn detection: %w", tErr)
			}
			g = &group{
				burn: chain.Burn{
					TxID:      row.TrxID,
					Chain:     Chain,
					Source:    row.Act.Data.From,
					Height:    row.BlockNum,
					Timestamp: ts,
				},
				seen: map[uint64]bool{},
			}
			groups[key] = g
		}
		if g.seen[row.GlobalSequence] {
			continue
		}
		g.seen[row.GlobalSequence] = true
		g.burn.Amount += units
		g.raws = append(g.raws, raw)
	}

	burns := make([]chain.Burn, 0, len(groups))
	for _, g := range groups {
		raw, err := json.Marshal(g.raws)
		if err != nil {
			return nil, fmt.Errorf("eos: burn detection: %w", err)
		}
		g.burn.Raw = raw
		burns = append(burns, g.burn)
	}
	sort.Slice(burns, func(i, j int) bool {
		if burns[i].Height != burns[j].Height {
			return burns[i].Height < burns[j].Height
		}
		return burns[i].TxID < burns[j].TxID
	})
	return burns, nil
}
