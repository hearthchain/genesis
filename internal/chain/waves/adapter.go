package waves

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/wavesplatform/gowaves/pkg/proto"

	"github.com/hearthchain/genesis/internal/chain"
)

// PrimaryNode is the read surface the adapter needs from the primary node;
// *Client and the file-backed fixture node both satisfy it.
type PrimaryNode interface {
	AllTransactions(ctx context.Context, addr string) ([]json.RawMessage, error)
	Height(ctx context.Context) (uint64, error)
	BalanceAfterConfirmations(ctx context.Context, addr string, confirmations uint64) (uint64, error)
}

// Adapter implements the chain port for Waves mainnet over two independent
// public nodes.
type Adapter struct {
	Primary     PrimaryNode
	Secondary   Node
	BurnAddress string
}

// Name returns the chain slug.
func (a *Adapter) Name() string { return "waves" }

// ValidateAddress rejects strings that are not a Waves mainnet address.
func (a *Adapter) ValidateAddress(addr string) error {
	parsed, err := proto.NewAddressFromString(addr)
	if err != nil {
		return err
	}
	if ok, vErr := parsed.Valid(proto.MainNetScheme); vErr != nil || !ok {
		return fmt.Errorf("not a Waves mainnet address")
	}
	return nil
}

// Height returns the primary node's chain height.
func (a *Adapter) Height(ctx context.Context) (uint64, error) { return a.Primary.Height(ctx) }

// BurnCandidates detects the burns inside the window from the burn address's
// transaction history on the primary node.
func (a *Adapter) BurnCandidates(ctx context.Context, window chain.Window) ([]chain.Burn, error) {
	txs, err := a.Primary.AllTransactions(ctx, a.BurnAddress)
	if err != nil {
		return nil, err
	}
	return DetectBurns(txs, a.BurnAddress, window)
}

// CrossCheck compares the burn against the secondary node.
func (a *Adapter) CrossCheck(ctx context.Context, burn chain.Burn, confirmations uint64) (chain.Verdict, error) {
	return CrossCheck(ctx, a.Secondary, burn, confirmations)
}

// Deltas replays raw history rows into signed balance changes.
func (a *Adapter) Deltas(txs []json.RawMessage, addr string) ([]chain.Delta, chain.Status) {
	return Deltas(txs, addr)
}

// History fetches the source's full transaction history and verifies the
// balance invariant: the balance recomputed from deltas up to the reference
// height must exactly equal the node-reported balance at that height. A wrong
// credit is unrecoverable; a blocked address is recoverable, so any doubt
// resolves to unsupported. Waves nodes serve history from genesis, so no
// opening layer is ever synthesized.
func (a *Adapter) History(ctx context.Context, source string, reference, tip uint64) (chain.History, error) {
	txs, err := a.Primary.AllTransactions(ctx, source)
	if err != nil {
		return chain.History{}, err
	}
	sortByHeightAscending(txs)
	h := chain.History{Address: source, Txs: txs, ReferenceHeight: reference}
	deltas, deltaStatus := Deltas(txs, source)
	if deltaStatus.Kind != chain.StatusOK {
		h.Status, h.Reason = chain.StatusUnsupported, deltaStatus.Reason
		return h, nil
	}
	var sum int64
	for _, d := range deltas {
		if d.Height <= reference {
			sum += d.Amount
		}
	}
	h.Recomputed = sum
	balance, err := a.Primary.BalanceAfterConfirmations(ctx, source, tip-reference)
	if err != nil {
		h.Status, h.Reason = chain.StatusUnsupported, fmt.Sprintf("balance fetch failed: %v", err)
		return h, nil
	}
	h.NodeBalance = balance
	if sum < 0 || uint64(sum) != balance {
		h.Status = chain.StatusUnsupported
		h.Reason = fmt.Sprintf("recomputed %d != node balance %d at height %d", sum, balance, reference)
		return h, nil
	}
	h.Status = chain.StatusOK
	return h, nil
}

func sortByHeightAscending(txs []json.RawMessage) {
	height := func(raw json.RawMessage) uint64 {
		var peek struct {
			Height uint64 `json:"height"`
		}
		_ = json.Unmarshal(raw, &peek) // undecodable rows sort first and fail later checks
		return peek.Height
	}
	sort.SliceStable(txs, func(i, j int) bool { return height(txs[i]) < height(txs[j]) })
}
