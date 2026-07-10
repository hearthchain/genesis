package eos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hearthchain/genesis/internal/chain"
)

// ChainAPI is the finality/balance/account surface of a chain API node;
// *Client and the fixture source satisfy it.
type ChainAPI interface {
	LastIrreversibleBlock(ctx context.Context) (uint64, error)
	CombinedBalance(ctx context.Context, account string) (uint64, error)
	AccountCreated(ctx context.Context, account string) (time.Time, error)
}

// HistoryAPI is the action-history surface; *Hyperion and the fixture source
// satisfy it.
type HistoryAPI interface {
	TransferActions(ctx context.Context, account string, maxActions int) ([]json.RawMessage, error)
	TransfersTo(ctx context.Context, account string, maxActions int) ([]json.RawMessage, error)
}

// maxHistoryActions caps how much history one source account may have before
// it goes to manual review (exchanges and contracts, not holders).
const maxHistoryActions = 50_000

// hyperionFloorDate is the wall-clock date of block 300,000,000, the first
// block the free public index covers (EOS Rio's first_indexed_block);
// nothing older is retrievable from any free source. Balances that
// predate the index enter as one opening layer dated here: pricing since
// 2023-03-18 can only understate against true deeper history, so the credit
// stays a floor. A deep-history upgrade can raise it later, never lower it.
func hyperionFloorDate() time.Time {
	return time.Date(2023, 3, 18, 0, 0, 0, 0, time.UTC)
}

// BuildHistory fetches and verifies one account's transfer history. The
// public index is truncated, so instead of the Waves-style exact invariant
// the remainder between the live combined balance and the replayed deltas
// becomes the synthetic opening layer. Soundness rests on four legs: balance
// and history come from independent operators; the balance is read before
// and after the history fetch (with one retry) so nothing moved in between;
// a negative remainder blocks the account; and an account created after the
// index floor must have no remainder at all.
func BuildHistory(
	ctx context.Context, api ChainAPI, hist HistoryAPI, account string, reference uint64,
) (chain.History, error) {
	h := chain.History{Address: account, ReferenceHeight: reference}
	const attempts = 2
	for range attempts {
		balanceBefore, err := api.CombinedBalance(ctx, account)
		if err != nil {
			return h, err
		}
		txs, err := hist.TransferActions(ctx, account, maxHistoryActions)
		if errors.Is(err, ErrHistoryTooLarge) {
			h.Status, h.Reason = chain.StatusUnsupported, err.Error()
			return h, nil
		}
		if err != nil {
			return h, err
		}
		balanceAfter, err := api.CombinedBalance(ctx, account)
		if err != nil {
			return h, err
		}
		if balanceBefore != balanceAfter {
			continue
		}
		return verifyHistory(ctx, api, h, account, txs, balanceBefore)
	}
	h.Status, h.Reason = chain.StatusUnsupported, "balance moved during history fetch; retry later"
	return h, nil
}

func verifyHistory(
	ctx context.Context, api ChainAPI, h chain.History, account string, txs []json.RawMessage, balance uint64,
) (chain.History, error) {
	h.Txs = txs
	h.NodeBalance = balance
	deltas, status := Deltas(txs, account)
	if status.Kind != chain.StatusOK {
		h.Status, h.Reason = chain.StatusUnsupported, status.Reason
		return h, nil
	}
	var sum int64
	for _, d := range deltas {
		sum += d.Amount
	}
	h.Recomputed = sum
	// #nosec G115 -- supply-bounded, orders below int64
	opening := int64(balance) - sum
	if opening < 0 {
		h.Status = chain.StatusUnsupported
		h.Reason = fmt.Sprintf("recomputed sum %d exceeds combined balance %d", sum, balance)
		return h, nil
	}
	if opening > 0 {
		created, err := api.AccountCreated(ctx, account)
		if err != nil {
			return h, err
		}
		if created.After(hyperionFloorDate()) {
			h.Status = chain.StatusUnsupported
			h.Reason = fmt.Sprintf(
				"account created after the index floor yet %d base units are unexplained by its history", opening)
			return h, nil
		}
		h.OpeningBaseUnits = uint64(opening)
		h.OpeningAt = hyperionFloorDate()
	}
	h.Status = chain.StatusOK
	return h, nil
}
