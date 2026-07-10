// Package chain defines the chain-adapter port: the chain-agnostic types the
// watcher, credit engine and snapshot work with. One package per chain (waves
// first) implements the concrete detection and history extraction.
package chain

import (
	"context"
	"encoding/json"
	"time"
)

// Adapter is the per-chain port the watcher and the API consume: everything
// chain-specific (node protocols, tx decoding, invariants) lives behind it.
type Adapter interface {
	// Name is the chain slug used in artifacts, routes and config ("waves").
	Name() string
	// ValidateAddress rejects strings that are not a source address (or
	// account name) on this chain's mainnet.
	ValidateAddress(addr string) error
	// Height returns the finalized tip the confirmation rule counts from:
	// the node height on Waves, the last irreversible block on EOS.
	Height(ctx context.Context) (uint64, error)
	// BurnCandidates lists the burns detected inside the window, mature or
	// not; confirmation depth is the watcher's call.
	BurnCandidates(ctx context.Context, window Window) ([]Burn, error)
	// CrossCheck re-fetches a burn from the independent secondary source
	// and compares the canonical fields.
	CrossCheck(ctx context.Context, burn Burn, confirmations uint64) (Verdict, error)
	// History fetches and verifies the source's transfer history; Status
	// "ok" is required before any credit is computed.
	History(ctx context.Context, source string, reference, tip uint64) (History, error)
	// Deltas replays raw history rows into signed balance changes; it must
	// reproduce a History's Recomputed sum from its Txs.
	Deltas(txs []json.RawMessage, addr string) ([]Delta, Status)
}

// Window bounds a burn campaign in block heights, inclusive on both ends.
type Window struct {
	Start uint64 `json:"startHeight"`
	End   uint64 `json:"endHeight"`
}

// Burn is one detected burn: a transfer of the native coin to the published
// burn address, attributed to the sending address.
type Burn struct {
	TxID      string          `json:"txId"`
	Chain     string          `json:"chain"`
	Source    string          `json:"source"`
	Amount    uint64          `json:"amountBaseUnits"`
	Height    uint64          `json:"height"`
	Timestamp time.Time       `json:"timestamp"`
	Raw       json.RawMessage `json:"raw"`
}

// Delta is one signed native-coin balance change of an address.
type Delta struct {
	TxID      string    `json:"txId"`
	Height    uint64    `json:"height"`
	Timestamp time.Time `json:"timestamp"`
	Amount    int64     `json:"amount"`
}

// The two verdicts of history verification: anything that is not provably
// "ok" is "unsupported" and blocks the address to manual review.
const (
	StatusOK          = "ok"
	StatusUnsupported = "unsupported"
)

// BindingSource is the optional adapter capability of chains whose bindings
// ride on-chain transfer memos instead of API-submitted signatures.
type BindingSource interface {
	// MemoBindings lists the valid binding memos carried by transfers to the
	// burn account from fromHeight on, in ascending chain order (latest
	// wins downstream). No upper bound: bindings stay open after the burn
	// window closes.
	MemoBindings(ctx context.Context, fromHeight uint64) ([]MemoBinding, error)
	// CrossCheckBinding verifies one memo binding against the independent
	// secondary source before it may enter the registry.
	CrossCheckBinding(ctx context.Context, mb MemoBinding) (Verdict, error)
}

// MemoBinding is one on-chain binding statement: the transfer carrying it is
// signed by the source account's key, which is the whole proof.
type MemoBinding struct {
	Source    string
	Hearth    string
	TxID      string
	Height    uint64
	Timestamp time.Time
	Raw       json.RawMessage // the carrying transfer row, for cross-check
}

// WithOpening prepends the synthetic opening delta of a truncated history:
// the pre-index remainder enters as one deposit dated at the truncation
// boundary, opening the oldest layer. Height 0 keeps it first under any
// height-ordered replay, and the "opening" TxID never appears in burn maps.
// A zero opening returns the deltas unchanged (complete-history chains).
func WithOpening(deltas []Delta, opening uint64, at time.Time) []Delta {
	if opening == 0 {
		return deltas
	}
	// #nosec G115 -- chain supplies fit int64 by orders of magnitude
	head := Delta{TxID: "opening", Timestamp: at, Amount: int64(opening)}
	return append([]Delta{head}, deltas...)
}

// Status is the verdict of a delta reconstruction: Kind "ok" or "unsupported"
// (the history contains a transaction the adapter does not interpret; the
// address is blocked to manual review rather than risking a wrong credit).
type Status struct {
	Kind   string
	Reason string
}

// Verdict is the outcome of a double-source check: confirmed, mismatch (with
// the diverging field names) or pending_crosscheck while the second source has
// not yet buried the burn under enough confirmations.
type Verdict struct {
	Status     string   `json:"status"`
	Node       string   `json:"node,omitempty"`
	Mismatches []string `json:"mismatchFields,omitempty"`
}

// History is the fetched and verified transfer history of one address
// together with the safety-invariant verdict. Status is "ok" only when the
// balance recomputed from Txs exactly matches the node-reported balance at
// ReferenceHeight. On chains whose public history is truncated (EOS), the
// pre-index remainder is a synthetic opening layer dated OpeningAt; zero
// OpeningBaseUnits means the history is complete from genesis.
type History struct {
	Address          string
	Txs              []json.RawMessage // verbatim source rows, ascending height
	ReferenceHeight  uint64
	NodeBalance      uint64
	Recomputed       int64
	OpeningBaseUnits uint64
	OpeningAt        time.Time
	Status           string
	Reason           string
}
