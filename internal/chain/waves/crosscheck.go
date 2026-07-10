package waves

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hearthchain/genesis/internal/chain"
)

// canonicalTx is the field set compared across the two nodes. Formatting and
// node-specific extras are irrelevant; these fields define the burn.
type canonicalTx struct {
	ID        string  `json:"id"`
	Type      int     `json:"type"`
	Sender    string  `json:"sender"`
	Recipient string  `json:"recipient"`
	AssetID   *string `json:"assetId"`
	Amount    uint64  `json:"amount"`
	Fee       uint64  `json:"fee"`
	Timestamp int64   `json:"timestamp"`
	Height    uint64  `json:"height"`
	Transfers []struct {
		Recipient string `json:"recipient"`
		Amount    uint64 `json:"amount"`
	} `json:"transfers"`
}

// Node is the read surface CrossCheck needs from the secondary source; both
// *Client and test or fixture fakes satisfy it.
type Node interface {
	Height(ctx context.Context) (uint64, error)
	TransactionInfo(ctx context.Context, id string) (json.RawMessage, error)
}

// CrossCheck re-fetches a burn from the secondary node and compares the
// canonical fields. A lagging secondary is not a mismatch: the burn stays
// pending and is retried on the next poll.
func CrossCheck(ctx context.Context, secondary Node, burn chain.Burn, confirmations uint64) (chain.Verdict, error) {
	tip, err := secondary.Height(ctx)
	if err != nil {
		return chain.Verdict{}, err
	}
	if tip < burn.Height+confirmations {
		return chain.Verdict{Status: "pending_crosscheck"}, nil
	}
	theirs, err := secondary.TransactionInfo(ctx, burn.TxID)
	if err != nil {
		return chain.Verdict{}, err
	}
	mismatches, err := compareCanonical(burn.Raw, theirs)
	if err != nil {
		return chain.Verdict{}, err
	}
	if len(mismatches) > 0 {
		return chain.Verdict{Status: "mismatch", Mismatches: mismatches}, nil
	}
	return chain.Verdict{Status: "confirmed"}, nil
}

func compareCanonical(ours, theirs json.RawMessage) ([]string, error) {
	var a, b canonicalTx
	if err := json.Unmarshal(ours, &a); err != nil {
		return nil, fmt.Errorf("waves: cross-check: our record: %w", err)
	}
	if err := json.Unmarshal(theirs, &b); err != nil {
		return nil, fmt.Errorf("waves: cross-check: secondary record: %w", err)
	}
	var out []string
	add := func(field string, differs bool) {
		if differs {
			out = append(out, field)
		}
	}
	add("id", a.ID != b.ID)
	add("type", a.Type != b.Type)
	add("sender", a.Sender != b.Sender)
	add("recipient", a.Recipient != b.Recipient)
	add("assetId", !equalPtr(a.AssetID, b.AssetID))
	add("amount", a.Amount != b.Amount)
	add("fee", a.Fee != b.Fee)
	add("timestamp", a.Timestamp != b.Timestamp)
	add("height", a.Height != b.Height)
	add("transfers", !equalTransfers(a, b))
	return out, nil
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalTransfers(a, b canonicalTx) bool {
	if len(a.Transfers) != len(b.Transfers) {
		return false
	}
	for i := range a.Transfers {
		if a.Transfers[i] != b.Transfers[i] {
			return false
		}
	}
	return true
}
