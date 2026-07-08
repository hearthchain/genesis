// Package bindings maintains the append-only registry of cabinet bindings:
// source address -> Hearth address, each authenticated by a signature of the
// source address's key. The JSONL artifact is published; the latest valid
// binding per source wins at snapshot freeze.
package bindings

import (
	"fmt"
	"sync"
	"time"

	"github.com/hearthchain/burning-page/internal/binding"
	"github.com/hearthchain/burning-page/internal/hearthaddr"
	"github.com/hearthchain/burning-page/internal/store"
)

// formatEosMemo marks a binding proven by an on-chain transfer memo; only
// the watcher's trusted AddVerified path may write it.
const formatEosMemo = "eos-memo-v1"

// Record is one bindings.jsonl line.
type Record struct {
	Source    string `json:"source"`
	Chain     string `json:"chain"`
	Hearth    string `json:"hearth"`
	PublicKey string `json:"publicKey,omitempty"`
	Signature string `json:"signature,omitempty"`
	// Format selects the proof: "" or "raw" sign the plain message,
	// "keeper-v1" the Keeper signCustomData envelope, "eos-memo-v1" carries
	// no signature at all (the proof is the on-chain transfer in TxID).
	Format     string    `json:"format,omitempty"`
	TxID       string    `json:"txId,omitempty"`
	ReceivedAt time.Time `json:"receivedAt"`
}

// Registry is the in-memory latest-wins index over the JSONL artifact.
type Registry struct {
	mu           sync.RWMutex
	path         string
	hearthScheme byte
	bySource     map[string]Record
	seenTx       map[string]bool
}

// Load reads the artifact and indexes the latest binding per source. Records
// were signature-checked on submission; loading trusts the local artifact.
func Load(path string, hearthScheme byte) (*Registry, error) {
	records, err := store.ReadJSONL[Record](path)
	if err != nil {
		return nil, err
	}
	r := &Registry{
		path:         path,
		hearthScheme: hearthScheme,
		bySource:     make(map[string]Record, len(records)),
		seenTx:       make(map[string]bool, len(records)),
	}
	for _, rec := range records {
		r.bySource[rec.Source] = rec
		if rec.TxID != "" {
			r.seenTx[rec.TxID] = true
		}
	}
	return r, nil
}

// Add verifies a submitted binding and, when valid, appends and indexes it.
// The Format field selects what bytes the signature covers: the canonical
// message itself (bindsign, CLI) or the Keeper signCustomData v1 envelope.
// Memo formats are refused here: an API submission carries no on-chain proof.
func (r *Registry) Add(rec Record) error {
	var err error
	switch rec.Format {
	case "", "raw":
		err = binding.Verify(rec.Source, rec.Hearth, r.hearthScheme, rec.PublicKey, rec.Signature)
	case "keeper-v1":
		err = binding.VerifyKeeperV1(rec.Source, rec.Hearth, r.hearthScheme, rec.PublicKey, rec.Signature)
	case formatEosMemo:
		err = fmt.Errorf("bindings: format %q is only recorded from chain, not submitted", rec.Format)
	default:
		err = fmt.Errorf("bindings: unknown signature format %q", rec.Format)
	}
	if err != nil {
		return err
	}
	return r.append(rec)
}

// AddVerified appends a binding whose proof already happened on chain (a
// cross-checked transfer memo). Only the watcher calls this; the API's Add
// path refuses these formats. The Hearth address is still validated: a
// malformed destination must never enter the artifact.
func (r *Registry) AddVerified(rec Record) error {
	if err := hearthaddr.Validate(rec.Hearth, r.hearthScheme); err != nil {
		return fmt.Errorf("bindings: %w", err)
	}
	return r.append(rec)
}

func (r *Registry) append(rec Record) error {
	if rec.ReceivedAt.IsZero() {
		rec.ReceivedAt = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if aErr := store.AppendJSONL(r.path, rec); aErr != nil {
		return fmt.Errorf("bindings: %w", aErr)
	}
	r.bySource[rec.Source] = rec
	if rec.TxID != "" {
		r.seenTx[rec.TxID] = true
	}
	return nil
}

// SeenTx reports whether a binding carried by this transaction was already
// recorded, including bindings later superseded (append-only dedup).
func (r *Registry) SeenTx(txID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.seenTx[txID]
}

// Current returns the source's latest binding record.
func (r *Registry) Current(source string) (Record, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.bySource[source]
	return rec, ok
}

// HearthFor resolves the current Hearth address of a source address.
func (r *Registry) HearthFor(source string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.bySource[source]
	return rec.Hearth, ok
}

// Count reports how many source addresses currently hold a binding.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.bySource)
}

// SourcesFor lists the source addresses currently bound to a Hearth address.
func (r *Registry) SourcesFor(hearth string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for source, rec := range r.bySource {
		if rec.Hearth == hearth {
			out = append(out, source)
		}
	}
	return out
}
