package api

import (
	"math/big"
	"net/http"
	"path/filepath"

	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/snapshot"
	"github.com/hearthchain/genesis/internal/store"
)

// chainStats aggregates the burn artifact of one chain for the public
// counters. Wavelet totals are strings: they can exceed what JSON numbers
// carry losslessly.
type chainStats struct {
	BurnedBaseUnits  string         `json:"burnedBaseUnits"`
	PendingBaseUnits string         `json:"pendingBaseUnits"`
	BurnsByStatus    map[string]int `json:"burnsByStatus"`
}

// stats serves the front-page counters. Everything is recomputed from the
// artifacts per request, like the address endpoint: the server stays a cache.
func (s *Server) stats(w http.ResponseWriter, _ *http.Request) {
	snap, _, err := snapshot.Build(s.cfg.DataDir, s.journals, s.cfg.HearthSchemeByte())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "artifacts_error", err.Error())
		return
	}
	chains, err := s.chainTotals()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "artifacts_error", err.Error())
		return
	}
	total, ok := new(big.Int).SetString(snap.TotalCreditMicro, creditBase)
	if !ok {
		total = new(big.Int)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"totalCreditMicro": snap.TotalCreditMicro,
		"totalCredit":      microToDecimal(total),
		"merkleRoot":       snap.Root,
		"participants":     len(snap.Entries),
		"bindings":         s.registry.Count(),
		"pendingSources":   len(snap.PendingSources),
		"blockedSources":   len(snap.BlockedSources),
		"chains":           chains,
		"windows":          s.windows(),
	})
}

// windows lists each configured chain's burn window.
func (s *Server) windows() map[string]chain.Window {
	out := make(map[string]chain.Window, len(s.cfg.Chains))
	for name, cc := range s.cfg.Chains {
		out[name] = cc.Window
	}
	return out
}

// chainTotals folds the latest row per txId of burns.jsonl into per-chain
// sums: confirmed amounts, amounts still waiting for depth or cross-check,
// and a count per lifecycle status.
func (s *Server) chainTotals() (map[string]*chainStats, error) {
	rows, err := store.ReadJSONL[pendingBurnView](filepath.Join(s.cfg.DataDir, "burns.jsonl"))
	if err != nil {
		return nil, err
	}
	latest := make(map[string]pendingBurnView, len(rows))
	for _, row := range rows {
		latest[row.TxID] = row
	}
	type acc struct {
		burned, pending *big.Int
		byStatus        map[string]int
	}
	accs := map[string]*acc{}
	for _, row := range latest {
		a := accs[row.Chain]
		if a == nil {
			a = &acc{burned: new(big.Int), pending: new(big.Int), byStatus: map[string]int{}}
			accs[row.Chain] = a
		}
		a.byStatus[row.Status]++
		amount := new(big.Int).SetUint64(row.AmountBaseUnits)
		switch row.Status {
		case statusConfirmed:
			a.burned.Add(a.burned, amount)
		case "pending_confirmations", "pending_crosscheck":
			a.pending.Add(a.pending, amount)
		}
	}
	out := make(map[string]*chainStats, len(accs))
	for chain, a := range accs {
		out[chain] = &chainStats{
			BurnedBaseUnits:  a.burned.String(),
			PendingBaseUnits: a.pending.String(),
			BurnsByStatus:    a.byStatus,
		}
	}
	return out, nil
}
