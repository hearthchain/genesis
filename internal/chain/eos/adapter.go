package eos

import (
	"context"
	"encoding/json"

	"github.com/hearthchain/burning-page/internal/chain"
)

// maxBurnTargetActions caps the burn target's own action fetch; the burn
// account accumulates the whole campaign, so its cap is far above the
// per-source history cap.
const maxBurnTargetActions = 500_000

// Adapter implements the chain port for EOS mainnet over three independent
// operators: a chain API node (finality, balances, accounts), a Hyperion
// index (action history) and a legacy v1 history node (cross-check).
type Adapter struct {
	API          ChainAPI
	Index        HistoryAPI
	Secondary    Secondary
	BurnAccount  string
	HearthScheme byte
}

// Name returns the chain slug.
func (a *Adapter) Name() string { return Chain }

// ValidateAddress rejects strings that are not an EOS account name.
func (a *Adapter) ValidateAddress(addr string) error { return ValidateAccount(addr) }

// Height returns the last irreversible block: final under Savanna, so the
// configured confirmation depth on EOS is zero.
func (a *Adapter) Height(ctx context.Context) (uint64, error) {
	return a.API.LastIrreversibleBlock(ctx)
}

// BurnCandidates detects the burns inside the window from the burn account's
// received transfers.
func (a *Adapter) BurnCandidates(ctx context.Context, window chain.Window) ([]chain.Burn, error) {
	rows, err := a.Index.TransfersTo(ctx, a.BurnAccount, maxBurnTargetActions)
	if err != nil {
		return nil, err
	}
	return DetectBurns(rows, a.BurnAccount, window)
}

// CrossCheck compares the burn against the secondary source. Confirmation
// depth is irrelevant on EOS: the secondary itself reports irreversibility.
func (a *Adapter) CrossCheck(ctx context.Context, burn chain.Burn, _ uint64) (chain.Verdict, error) {
	return CrossCheck(ctx, a.Secondary, burn)
}

// History fetches and verifies the source's transfer history; the tip is
// unused because balances are only readable live (the double-read in
// BuildHistory anchors them instead).
func (a *Adapter) History(ctx context.Context, source string, reference, _ uint64) (chain.History, error) {
	return BuildHistory(ctx, a.API, a.Index, source, reference)
}

// Deltas replays raw history rows into signed combined-balance changes.
func (a *Adapter) Deltas(txs []json.RawMessage, addr string) ([]chain.Delta, chain.Status) {
	return Deltas(txs, addr)
}

// CrossCheckBinding verifies one memo binding against the secondary source.
func (a *Adapter) CrossCheckBinding(ctx context.Context, mb chain.MemoBinding) (chain.Verdict, error) {
	return CrossCheckBinding(ctx, a.Secondary, mb)
}

// MemoBindings lists the valid binding memos carried by transfers to the
// burn account from fromHeight on, ascending.
func (a *Adapter) MemoBindings(ctx context.Context, fromHeight uint64) ([]chain.MemoBinding, error) {
	rows, err := a.Index.TransfersTo(ctx, a.BurnAccount, maxBurnTargetActions)
	if err != nil {
		return nil, err
	}
	all := ExtractBindings(rows, a.BurnAccount, a.HearthScheme)
	out := make([]chain.MemoBinding, 0, len(all))
	for _, b := range all {
		if b.Height >= fromHeight {
			out = append(out, b)
		}
	}
	return out, nil
}
