package eos

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hearthchain/burning-page/internal/chain"
)

// The two token contracts whose transfers move the combined native balance.
const (
	contractLegacy = "eosio.token"
	contractVaulta = "core.vaulta"
)

// actionRow is the projection of a Hyperion get_actions row the delta and
// burn rules read. Raw rows are kept verbatim in the artifacts; this struct
// only interprets them.
type actionRow struct {
	Timestamp      string `json:"@timestamp"`
	BlockNum       uint64 `json:"block_num"`
	TrxID          string `json:"trx_id"`
	GlobalSequence uint64 `json:"global_sequence"`
	Act            struct {
		Account string `json:"account"`
		Name    string `json:"name"`
		Data    struct {
			From     string      `json:"from"`
			To       string      `json:"to"`
			Quantity string      `json:"quantity"`
			Amount   json.Number `json:"amount"`
			Symbol   string      `json:"symbol"`
			Memo     string      `json:"memo"`
		} `json:"data"`
	} `json:"act"`
}

// decodeAction interprets one raw row, rejecting anything that is not a
// well-formed native-token transfer: a row we cannot interpret means the
// whole history is blocked, never guessed at.
func decodeAction(raw json.RawMessage) (actionRow, uint64, error) {
	var row actionRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return row, 0, fmt.Errorf("eos: undecodable action: %w", err)
	}
	if row.Act.Account != contractLegacy && row.Act.Account != contractVaulta {
		return row, 0, fmt.Errorf("eos: action %s from foreign contract %s", row.TrxID, row.Act.Account)
	}
	if row.Act.Name != "transfer" {
		return row, 0, fmt.Errorf("eos: action %s is %s, not a transfer", row.TrxID, row.Act.Name)
	}
	if row.TrxID == "" || row.BlockNum == 0 {
		return row, 0, fmt.Errorf("eos: action without trx_id or block_num")
	}
	units, err := rowUnits(row)
	if err != nil {
		return row, 0, err
	}
	return row, units, nil
}

// rowUnits extracts the transferred base units. Hyperion serves transfers
// either with the original "quantity" asset string or normalized into a
// numeric "amount" plus "symbol"; both parse exactly, never through floats.
func rowUnits(row actionRow) (uint64, error) {
	if row.Act.Data.Quantity != "" {
		return ParseQuantity(row.Act.Data.Quantity)
	}
	if row.Act.Data.Symbol == "" {
		return 0, fmt.Errorf("eos: action %s carries neither quantity nor amount+symbol", row.TrxID)
	}
	literal := row.Act.Data.Amount.String()
	whole, frac, _ := strings.Cut(literal, ".")
	if len(frac) > decimals {
		return 0, fmt.Errorf("eos: action %s amount %q exceeds %d decimals", row.TrxID, literal, decimals)
	}
	frac += strings.Repeat("0", decimals-len(frac))
	return ParseQuantity(whole + "." + frac + " " + row.Act.Data.Symbol)
}

// rowTime parses Hyperion's zone-less UTC timestamps (RFC3339 tolerated).
func rowTime(value string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02T15:04:05.000", "2006-01-02T15:04:05", time.RFC3339} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("eos: unparseable timestamp %q", value)
}

// Deltas reconstructs the signed combined-balance changes of an account from
// its Hyperion transfer rows. Rows are deduplicated by global sequence and
// netted per transaction across both token contracts, so a core.vaulta swap
// (A out, EOS in within one transaction) is a zero delta and layers.Build
// sees exactly one delta per transaction, mirroring the Waves rule.
func Deltas(txs []json.RawMessage, addr string) ([]chain.Delta, chain.Status) {
	rows := make([]actionRow, 0, len(txs))
	units := make([]uint64, 0, len(txs))
	seen := map[uint64]bool{}
	for _, raw := range txs {
		row, u, err := decodeAction(raw)
		if err != nil {
			return nil, chain.Status{Kind: chain.StatusUnsupported, Reason: err.Error()}
		}
		if seen[row.GlobalSequence] {
			continue // Hyperion pages can overlap; every action appears once
		}
		seen[row.GlobalSequence] = true
		rows = append(rows, row)
		units = append(units, u)
	}
	order := make([]int, len(rows))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return rows[order[a]].GlobalSequence < rows[order[b]].GlobalSequence
	})

	byTrx := map[string]int{}
	var deltas []chain.Delta
	for _, i := range order {
		row, u := rows[i], units[i]
		var signed int64
		if row.Act.Data.To == addr {
			signed += int64(u) // #nosec G115 -- supply-bounded, orders below int64
		}
		if row.Act.Data.From == addr {
			signed -= int64(u) // #nosec G115 -- supply-bounded, orders below int64
		}
		if at, ok := byTrx[row.TrxID]; ok {
			deltas[at].Amount += signed
			continue
		}
		ts, err := rowTime(row.Timestamp)
		if err != nil {
			return nil, chain.Status{Kind: chain.StatusUnsupported, Reason: err.Error()}
		}
		byTrx[row.TrxID] = len(deltas)
		deltas = append(deltas, chain.Delta{
			TxID:      row.TrxID,
			Height:    row.BlockNum,
			Timestamp: ts,
			Amount:    signed,
		})
	}
	return deltas, chain.Status{Kind: chain.StatusOK}
}
