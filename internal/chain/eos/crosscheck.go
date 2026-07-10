package eos

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/hearthchain/genesis/internal/binding"
	"github.com/hearthchain/genesis/internal/chain"
)

// Secondary is the read surface CrossCheck needs from the second source;
// *Greymass and test fakes satisfy it.
type Secondary interface {
	GetTransaction(ctx context.Context, id string) (TransactionInfo, error)
}

// CrossCheck re-fetches a burn's transaction from the independent secondary
// source and compares the canonical fields: block height, the multiset of
// native transfers from the source to the burn account (contract, quantity,
// memo), and the summed amount. Finality comes from the response itself
// (irreversible under Savanna), so no confirmation depth is involved. A
// still-reversible transaction is not a mismatch: the burn stays pending and
// is retried on the next poll.
func CrossCheck(ctx context.Context, secondary Secondary, burn chain.Burn) (chain.Verdict, error) {
	info, err := secondary.GetTransaction(ctx, burn.TxID)
	if err != nil {
		return chain.Verdict{}, err
	}
	if !info.Irreversible {
		return chain.Verdict{Status: "pending_crosscheck"}, nil
	}
	mismatches, err := compareCanonical(burn, info)
	if err != nil {
		return chain.Verdict{}, err
	}
	if len(mismatches) > 0 {
		return chain.Verdict{Status: "mismatch", Mismatches: mismatches}, nil
	}
	return chain.Verdict{Status: "confirmed"}, nil
}

// CrossCheckBinding verifies a memo binding against the secondary source:
// the transaction must be irreversible there and carry a transfer from the
// binding's source to some account with exactly the canonical binding memo.
// A missing or different memo is a mismatch; a reversible transaction stays
// pending and is retried on the next poll.
func CrossCheckBinding(ctx context.Context, secondary Secondary, mb chain.MemoBinding) (chain.Verdict, error) {
	info, err := secondary.GetTransaction(ctx, mb.TxID)
	if err != nil {
		return chain.Verdict{}, err
	}
	if !info.Irreversible {
		return chain.Verdict{Status: "pending_crosscheck"}, nil
	}
	var mismatches []string
	if info.ID != mb.TxID {
		mismatches = append(mismatches, "id")
	}
	if info.BlockNum != mb.Height {
		mismatches = append(mismatches, "height")
	}
	want := string(binding.Message(mb.Source, mb.Hearth))
	found := false
	for _, tr := range info.Transfers {
		if tr.From == mb.Source && tr.Memo == want {
			found = true
			break
		}
	}
	if !found {
		mismatches = append(mismatches, "memo")
	}
	if len(mismatches) > 0 {
		return chain.Verdict{Status: "mismatch", Mismatches: mismatches}, nil
	}
	return chain.Verdict{Status: "confirmed"}, nil
}

func compareCanonical(burn chain.Burn, info TransactionInfo) ([]string, error) {
	var out []string
	add := func(field string, differs bool) {
		if differs {
			out = append(out, field)
		}
	}
	add("id", info.ID != burn.TxID)
	add("height", info.BlockNum != burn.Height)

	ours, burnTo, err := recordedTransfers(burn)
	if err != nil {
		return nil, err
	}
	var theirs []string
	var theirSum uint64
	for _, tr := range info.Transfers {
		if tr.From != burn.Source || tr.To != burnTo {
			continue
		}
		units, uErr := ParseQuantity(tr.Quantity)
		if uErr != nil {
			return nil, uErr
		}
		theirSum += units
		theirs = append(theirs, fmt.Sprintf("%s|%s|%s|%d|%s", tr.Contract, tr.From, tr.To, units, tr.Memo))
	}
	add("amount", theirSum != burn.Amount)
	sort.Strings(ours)
	sort.Strings(theirs)
	add("transfers", !equalStrings(ours, theirs))
	return out, nil
}

// recordedTransfers canonicalizes the action rows stored in Burn.Raw.
func recordedTransfers(burn chain.Burn) ([]string, string, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(burn.Raw, &raws); err != nil {
		return nil, "", fmt.Errorf("eos: cross-check: our record: %w", err)
	}
	var out []string
	var burnTo string
	for _, raw := range raws {
		row, units, err := decodeAction(raw)
		if err != nil {
			return nil, "", fmt.Errorf("eos: cross-check: our record: %w", err)
		}
		burnTo = row.Act.Data.To
		out = append(out, fmt.Sprintf("%s|%s|%s|%d|%s",
			row.Act.Account, row.Act.Data.From, row.Act.Data.To, units, row.Act.Data.Memo))
	}
	return out, burnTo, nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
