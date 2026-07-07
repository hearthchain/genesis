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
	"github.com/hearthchain/burning-page/internal/store"
)

// Record is one bindings.jsonl line.
type Record struct {
	Source     string    `json:"source"`
	Chain      string    `json:"chain"`
	Hearth     string    `json:"hearth"`
	PublicKey  string    `json:"publicKey"`
	Signature  string    `json:"signature"`
	ReceivedAt time.Time `json:"receivedAt"`
}

// Registry is the in-memory latest-wins index over the JSONL artifact.
type Registry struct {
	mu           sync.RWMutex
	path         string
	hearthScheme byte
	bySource     map[string]Record
}

// Load reads the artifact and indexes the latest binding per source. Records
// were signature-checked on submission; loading trusts the local artifact.
func Load(path string, hearthScheme byte) (*Registry, error) {
	records, err := store.ReadJSONL[Record](path)
	if err != nil {
		return nil, err
	}
	r := &Registry{path: path, hearthScheme: hearthScheme, bySource: make(map[string]Record, len(records))}
	for _, rec := range records {
		r.bySource[rec.Source] = rec
	}
	return r, nil
}

// Add verifies a submitted binding and, when valid, appends and indexes it.
func (r *Registry) Add(rec Record) error {
	if err := binding.Verify(rec.Source, rec.Hearth, r.hearthScheme, rec.PublicKey, rec.Signature); err != nil {
		return err
	}
	if rec.ReceivedAt.IsZero() {
		rec.ReceivedAt = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := store.AppendJSONL(r.path, rec); err != nil {
		return fmt.Errorf("bindings: %w", err)
	}
	r.bySource[rec.Source] = rec
	return nil
}

// HearthFor resolves the current Hearth address of a source address.
func (r *Registry) HearthFor(source string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.bySource[source]
	return rec.Hearth, ok
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
