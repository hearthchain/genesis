// Package watcher runs the burn-window loop: detect burns via the chain
// adapter, cross-check each against the independent secondary source, and
// persist burn records plus the source-address transfer histories the credit
// formula needs. Every poll rescans the whole window and skips what is
// already recorded, so the watcher is idempotent and crash-safe with no state
// beyond the artifacts.
package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hearthchain/burning-page/internal/chain"
	"github.com/hearthchain/burning-page/internal/config"
	"github.com/hearthchain/burning-page/internal/store"
)

// BurnRecord is one burns.jsonl line; a later line with the same TxID
// supersedes an earlier one (pending burns get re-checked on later polls).
type BurnRecord struct {
	chain.Burn
	Status     string        `json:"status"`
	CrossCheck chain.Verdict `json:"crossCheck"`
	DetectedAt time.Time     `json:"detectedAt"`
}

// Watcher polls one chain for burns through its adapter.
type Watcher struct {
	Adapter  chain.Adapter
	ChainCfg config.ChainConfig
	DataDir  string
}

// Poll performs one full-window pass.
func (w *Watcher) Poll(ctx context.Context) error {
	tip, err := w.Adapter.Height(ctx)
	if err != nil {
		return err
	}
	if tip <= w.ChainCfg.Confirmations {
		return nil
	}
	burns, err := w.Adapter.BurnCandidates(ctx, w.ChainCfg.Window)
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

func (w *Watcher) burnsPath() string { return filepath.Join(w.DataDir, "burns.jsonl") }

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
	if b.Height+w.ChainCfg.Confirmations > tip {
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
	verdict, err := w.Adapter.CrossCheck(ctx, b, w.ChainCfg.Confirmations)
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
	path := filepath.Join(w.DataDir, "transfers", w.Adapter.Name(), source+".jsonl")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	reference := min(w.ChainCfg.Window.End, tip-w.ChainCfg.Confirmations)
	h, err := w.Adapter.History(ctx, source, reference, tip)
	if err != nil {
		return err
	}
	meta := store.TransferMeta{
		Address:          source,
		Chain:            w.Adapter.Name(),
		FetchedAt:        time.Now().UTC(),
		ReferenceHeight:  h.ReferenceHeight,
		NodeBalance:      h.NodeBalance,
		Recomputed:       h.Recomputed,
		OpeningBaseUnits: h.OpeningBaseUnits,
		OpeningAt:        h.OpeningAt,
		Status:           h.Status,
		Reason:           h.Reason,
	}
	slog.Info("history recorded", "source", source, "status", meta.Status, "reason", meta.Reason)
	return store.WriteTransfers(path, meta, h.Txs)
}
