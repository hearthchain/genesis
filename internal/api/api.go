// Package api serves the three read/submit endpoints of the burn backend:
// a live credit preview by source address, the cabinet balance by Hearth
// address, and binding submission. The server is never authoritative: every
// number it returns is recomputable from the published artifacts.
package api

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"path/filepath"

	"github.com/hearthchain/burning-page/internal/binding"
	"github.com/hearthchain/burning-page/internal/bindings"
	"github.com/hearthchain/burning-page/internal/chain"
	"github.com/hearthchain/burning-page/internal/chain/chains"
	"github.com/hearthchain/burning-page/internal/config"
	"github.com/hearthchain/burning-page/internal/credit"
	"github.com/hearthchain/burning-page/internal/evidence"
	"github.com/hearthchain/burning-page/internal/hearthaddr"
	"github.com/hearthchain/burning-page/internal/journal"
	"github.com/hearthchain/burning-page/internal/layers"
	"github.com/hearthchain/burning-page/internal/snapshot"
	"github.com/hearthchain/burning-page/internal/store"
)

const (
	maxPreviewConcurrency = 4 // preview is the only endpoint spending public-node quota
	maxBindingBodyBytes   = 4 << 10
	microPerCredit        = 1_000_000
	creditBase            = 10
	statusConfirmed       = "confirmed" // terminal burn status in burns.jsonl
	chainWaves            = "waves"
)

// Server wires the endpoints to their dependencies: one adapter and one
// price journal per configured chain.
type Server struct {
	adapters   map[string]chain.Adapter
	journals   map[string]*journal.Journal
	registry   *bindings.Registry
	cfg        config.Config
	previewSem chan struct{}
}

// New builds a Server.
func New(
	adapters map[string]chain.Adapter, journals map[string]*journal.Journal,
	reg *bindings.Registry, cfg config.Config,
) *Server {
	return &Server{
		adapters:   adapters,
		journals:   journals,
		registry:   reg,
		cfg:        cfg,
		previewSem: make(chan struct{}, maxPreviewConcurrency),
	}
}

//go:embed bind.html
var bindPage []byte

// Handler returns the HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/preview/{chain}/{address}", s.preview)
	mux.HandleFunc("GET /api/address/{hearth}", s.address)
	mux.HandleFunc("POST /api/bindings", s.postBinding)
	mux.HandleFunc("GET /api/stats", s.stats)
	mux.HandleFunc("GET /bind", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(bindPage)
	})
	return withCORS(s.cfg.AllowedOrigins, mux)
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	chainName := r.PathValue("chain")
	adapter, ok := s.adapters[chainName]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown_chain", "no such burn lane: "+chainName)
		return
	}
	addr := r.PathValue("address")
	if err := adapter.ValidateAddress(addr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_address", err.Error())
		return
	}
	select {
	case s.previewSem <- struct{}{}:
		defer func() { <-s.previewSem }()
	default:
		writeError(w, http.StatusTooManyRequests, "busy", "too many concurrent previews")
		return
	}
	h, err := s.previewHistory(r.Context(), adapter, chainName, addr)
	if err != nil {
		writeError(w, http.StatusBadGateway, "node_error", err.Error())
		return
	}
	if h.Status != chain.StatusOK {
		writeError(w, http.StatusUnprocessableEntity, "unsupported_history", h.Reason+"; manual review")
		return
	}
	deltas, status := adapter.Deltas(h.Txs, addr)
	if status.Kind != chain.StatusOK {
		writeError(w, http.StatusUnprocessableEntity, "unsupported_history", status.Reason+"; manual review")
		return
	}
	deltas = chain.WithOpening(deltas, h.OpeningBaseUnits, h.OpeningAt)
	profile, _, err := layers.Build(deltas, nil)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "unsupported_history", err.Error())
		return
	}
	units, err := chains.BaseUnits(chainName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_error", err.Error())
		return
	}
	total, perLayer, err := credit.Compute(profile, s.journals[chainName], units)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "unsupported_history", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"address":            addr,
		"status":             "ok",
		"layers":             perLayer,
		"minimumCreditMicro": total.String(),
		"minimumCredit":      microToDecimal(total),
	})
}

// previewHistory fetches the verified history at the freshest final state:
// the reference is the finalized tip minus the confirmation depth, capped by
// the window end when one is configured. The balance invariant runs here too,
// so a preview can never show a figure the snapshot would refuse to credit.
func (s *Server) previewHistory(
	ctx context.Context, adapter chain.Adapter, chainName, addr string,
) (chain.History, error) {
	tip, err := adapter.Height(ctx)
	if err != nil {
		return chain.History{}, err
	}
	cc := s.cfg.Chains[chainName]
	if tip <= cc.Confirmations {
		return chain.History{}, fmt.Errorf("chain tip %d not past the confirmation depth", tip)
	}
	reference := tip - cc.Confirmations
	if cc.Window.End > 0 && cc.Window.End < reference {
		reference = cc.Window.End
	}
	return adapter.History(ctx, addr, reference, tip)
}

func (s *Server) address(w http.ResponseWriter, r *http.Request) {
	hearth := r.PathValue("hearth")
	if err := hearthaddr.Validate(hearth, s.cfg.HearthSchemeByte()); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_address", err.Error())
		return
	}
	_, bundles, err := snapshot.Build(s.cfg.DataDir, s.journals, s.cfg.HearthSchemeByte())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "artifacts_error", err.Error())
		return
	}
	total := new(big.Int)
	burns := []any{}
	for _, b := range bundles {
		if b.Hearth != hearth {
			continue
		}
		burns = append(burns, confirmedBurnView{Bundle: b, Status: statusConfirmed})
		c, ok := new(big.Int).SetString(b.CreditMicro, creditBase)
		if ok {
			total.Add(total, c)
		}
	}
	pending, pErr := s.pendingBurns(hearth)
	if pErr != nil {
		writeError(w, http.StatusInternalServerError, "artifacts_error", pErr.Error())
		return
	}
	burns = append(burns, pending...)
	sources := s.registry.SourcesFor(hearth)
	if sources == nil {
		sources = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hearthAddress":      hearth,
		"minimumCreditMicro": total.String(),
		"minimumCredit":      microToDecimal(total),
		"bindings":           sources,
		"burns":              burns,
	})
}

func (s *Server) postBinding(w http.ResponseWriter, r *http.Request) {
	var rec bindings.Record
	body := http.MaxBytesReader(w, r.Body, maxBindingBodyBytes)
	if err := json.NewDecoder(body).Decode(&rec); err != nil {
		writeError(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	rec.Chain = chainWaves
	if err := s.registry.Add(rec); err != nil {
		writeBindingError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"accepted": true})
}

// confirmedBurnView decorates an evidence bundle with its lifecycle status.
type confirmedBurnView struct {
	evidence.Bundle
	Status string `json:"status"`
}

// pendingBurnView is a burn that is visible but not yet credited: detected on
// chain, waiting for confirmation depth or for the cross-check.
type pendingBurnView struct {
	TxID            string `json:"txId"`
	Chain           string `json:"chain"`
	Source          string `json:"source"`
	AmountBaseUnits uint64 `json:"amountBaseUnits"`
	Height          uint64 `json:"height"`
	Status          string `json:"status"`
}

// pendingBurns lists the not-yet-confirmed burns whose source is bound to the
// hearth address. The JSONL schema of burns.jsonl is the contract here.
func (s *Server) pendingBurns(hearth string) ([]any, error) {
	rows, err := store.ReadJSONL[pendingBurnView](filepath.Join(s.cfg.DataDir, "burns.jsonl"))
	if err != nil {
		return nil, err
	}
	latest := make(map[string]pendingBurnView, len(rows))
	for _, r := range rows {
		latest[r.TxID] = r
	}
	var out []any
	for _, r := range latest {
		if r.Status == statusConfirmed {
			continue
		}
		bound, ok := s.registry.HearthFor(r.Source)
		if !ok || bound != hearth {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func writeBindingError(w http.ResponseWriter, err error) {
	if errors.Is(err, binding.ErrBadSignature) || errors.Is(err, binding.ErrSourceMismatch) {
		writeError(w, http.StatusUnauthorized, "invalid_signature", err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_binding", err.Error())
}

// microToDecimal renders micro-HRTH as a decimal credit string, e.g.
// 49713174000 -> "49713.174000".
func microToDecimal(micro *big.Int) string {
	quo, rem := new(big.Int).QuoRem(micro, big.NewInt(microPerCredit), new(big.Int))
	return fmt.Sprintf("%s.%06d", quo.String(), rem.Int64())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
