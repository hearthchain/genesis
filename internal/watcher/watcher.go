// Package watcher runs the burn-window loop: detect burns on the primary
// node, cross-check each against the secondary, and persist burn records plus
// the source-address transfer histories the credit formula needs. Every poll
// rescans the whole window and skips what is already recorded, so the watcher
// is idempotent and crash-safe with no state beyond the artifacts.
package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hearthchain/burning-page/internal/chain"
	"github.com/hearthchain/burning-page/internal/chain/waves"
	"github.com/hearthchain/burning-page/internal/config"
	"github.com/hearthchain/burning-page/internal/store"
)

// Node is the read surface the watcher needs from a Waves node; *waves.Client
// and the file-backed fixture node both satisfy it.
type Node interface {
	AllTransactions(ctx context.Context, addr string) ([]json.RawMessage, error)
	Height(ctx context.Context) (uint64, error)
	BalanceAfterConfirmations(ctx context.Context, addr string, confirmations uint64) (uint64, error)
	TransactionInfo(ctx context.Context, id string) (json.RawMessage, error)
}

// BurnRecord is one burns.jsonl line; a later line with the same TxID
// supersedes an earlier one (pending burns get re-checked on later polls).
type BurnRecord struct {
	chain.Burn
	Status     string        `json:"status"`
	CrossCheck waves.Verdict `json:"crossCheck"`
	DetectedAt time.Time     `json:"detectedAt"`
}

const statusUnsupported = "unsupported"

// Watcher polls one chain (Waves mainnet) for burns.
type Watcher struct {
	Primary   Node
	Secondary Node
	Cfg       config.Config
}

// Poll performs one full-window pass.
func (w *Watcher) Poll(ctx context.Context) error {
	tip, err := w.Primary.Height(ctx)
	if err != nil {
		return err
	}
	if tip <= w.Cfg.Confirmations {
		return nil
	}
	txs, err := w.Primary.AllTransactions(ctx, w.Cfg.BurnAddress)
	if err != nil {
		return err
	}
	burns, err := waves.DetectBurns(txs, w.Cfg.BurnAddress, w.Cfg.Window)
	if err != nil {
		return err
	}
	latest, err := w.latestRecords()
	if err != nil {
		return err
	}
	for _, b := range burns {
		if pErr := w.processBurn(ctx, b, latest, tip); pErr != nil {
			return pErr
		}
	}
	return nil
}

func (w *Watcher) burnsPath() string { return filepath.Join(w.Cfg.DataDir, "burns.jsonl") }

func (w *Watcher) latestRecords() (map[string]BurnRecord, error) {
	records, err := store.ReadJSONL[BurnRecord](w.burnsPath())
	if err != nil {
		return nil, err
	}
	latest := make(map[string]BurnRecord, len(records))
	for _, r := range records {
		latest[r.TxID] = r
	}
	return latest, nil
}

func (w *Watcher) processBurn(ctx context.Context, b chain.Burn, latest map[string]BurnRecord, tip uint64) error {
	prev, seen := latest[b.TxID]
	if seen && (prev.Status == "confirmed" || prev.Status == "mismatch") {
		return nil
	}
	if b.Height+w.Cfg.Confirmations > tip {
		if seen {
			return nil // already visible as pending; nothing changed
		}
		rec := BurnRecord{Burn: b, Status: "pending_confirmations", DetectedAt: time.Now().UTC()}
		if aErr := store.AppendJSONL(w.burnsPath(), rec); aErr != nil {
			return aErr
		}
		slog.Info("burn recorded", "txId", b.TxID, "source", b.Source, "amount", b.Amount, "status", rec.Status)
		return nil
	}
	verdict, err := waves.CrossCheck(ctx, w.Secondary, b, w.Cfg.Confirmations)
	if err != nil {
		return err
	}
	if seen && verdict.Status == prev.Status {
		return nil // still waiting; no need to grow the artifact
	}
	rec := BurnRecord{Burn: b, Status: verdict.Status, CrossCheck: verdict, DetectedAt: time.Now().UTC()}
	if aErr := store.AppendJSONL(w.burnsPath(), rec); aErr != nil {
		return aErr
	}
	slog.Info("burn recorded", "txId", b.TxID, "source", b.Source, "amount", b.Amount, "status", verdict.Status)
	if verdict.Status != "confirmed" {
		return nil
	}
	return w.ensureHistory(ctx, b.Source, tip)
}

// ensureHistory fetches, verifies and persists the transfer history of a
// source address once; the artifact is the credit formula's input.
func (w *Watcher) ensureHistory(ctx context.Context, source string, tip uint64) error {
	path := filepath.Join(w.Cfg.DataDir, "transfers", source+".jsonl")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	txs, err := w.Primary.AllTransactions(ctx, source)
	if err != nil {
		return err
	}
	sortByHeightAscending(txs)
	reference := min(w.Cfg.Window.End, tip-w.Cfg.Confirmations)
	meta := store.TransferMeta{
		Address:         source,
		FetchedAt:       time.Now().UTC(),
		ReferenceHeight: reference,
	}
	res := w.invariant(ctx, txs, source, reference, tip)
	meta.Status, meta.Reason, meta.Recomputed, meta.NodeBalance = res.status, res.reason, res.sum, res.balance
	slog.Info("history recorded", "source", source, "status", meta.Status, "reason", meta.Reason)
	return store.WriteTransfers(path, meta, txs)
}

// invariant recomputes the balance from deltas up to the reference height and
// requires exact equality with the node-reported balance at that height.
// A wrong credit is unrecoverable; a blocked address is recoverable, so any
// doubt resolves to "unsupported".
type invariantResult struct {
	status  string
	reason  string
	sum     int64
	balance uint64
}

func (w *Watcher) invariant(
	ctx context.Context, txs []json.RawMessage, source string, reference, tip uint64,
) invariantResult {
	deltas, deltaStatus := waves.Deltas(txs, source)
	if deltaStatus.Kind != "ok" {
		return invariantResult{status: statusUnsupported, reason: deltaStatus.Reason}
	}
	var sum int64
	for _, d := range deltas {
		if d.Height <= reference {
			sum += d.Amount
		}
	}
	balance, err := w.Primary.BalanceAfterConfirmations(ctx, source, tip-reference)
	if err != nil {
		return invariantResult{status: statusUnsupported, reason: fmt.Sprintf("balance fetch failed: %v", err), sum: sum}
	}
	if sum < 0 || uint64(sum) != balance {
		reason := fmt.Sprintf("recomputed %d != node balance %d at height %d", sum, balance, reference)
		return invariantResult{status: statusUnsupported, reason: reason, sum: sum, balance: balance}
	}
	return invariantResult{status: "ok", sum: sum, balance: balance}
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
