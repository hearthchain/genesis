package eos

import (
	"encoding/json"
	"sort"

	"github.com/hearthchain/genesis/internal/binding"
	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/hearthaddr"
)

// ExtractBindings reads binding memos out of transfer rows to the burn
// account: a memo that is exactly `hearth-genesis-binding:v1:<from>:<hearth>`
// with a valid Hearth address binds the sender. The transfer itself is signed
// by the sender's active key on chain, so no further signature is verified
// here. Anything else (foreign memos, mismatched sources, undecodable rows)
// is silently ignored: not every transfer to the burn account is a binding.
// Rows come out in ascending chain order so the latest memo wins downstream.
func ExtractBindings(rows []json.RawMessage, burnAccount string, hearthScheme byte) []chain.MemoBinding {
	seen := map[uint64]bool{}
	var out []chain.MemoBinding
	for _, raw := range rows {
		row, _, err := decodeAction(raw)
		if err != nil || row.Act.Data.To != burnAccount || seen[row.GlobalSequence] {
			continue
		}
		seen[row.GlobalSequence] = true
		hearth, ok := bindingHearth(row, hearthScheme)
		if !ok {
			continue
		}
		ts, err := rowTime(row.Timestamp)
		if err != nil {
			continue
		}
		out = append(out, chain.MemoBinding{
			Source:    row.Act.Data.From,
			Hearth:    hearth,
			TxID:      row.TrxID,
			Height:    row.BlockNum,
			Timestamp: ts,
			Raw:       raw,
		})
	}
	sortBindingsAscending(out)
	return out
}

// bindingHearth checks the memo against the canonical statement for the
// row's sender and returns the bound Hearth address.
func bindingHearth(row actionRow, hearthScheme byte) (string, bool) {
	memo := row.Act.Data.Memo
	prefix := string(binding.Message(row.Act.Data.From, ""))
	if len(memo) <= len(prefix) || memo[:len(prefix)] != prefix {
		return "", false
	}
	hearth := memo[len(prefix):]
	if hearthaddr.Validate(hearth, hearthScheme) != nil {
		return "", false
	}
	return hearth, true
}

func sortBindingsAscending(bindings []chain.MemoBinding) {
	sort.SliceStable(bindings, func(i, j int) bool { return bindings[i].Height < bindings[j].Height })
}
