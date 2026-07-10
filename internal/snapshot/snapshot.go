// Package snapshot turns the artifacts (burns, transfers, bindings) into the
// deterministic credit snapshot: per-Hearth-address totals, a Merkle root over
// the sorted entries, and one evidence bundle per burn. Verify rebuilds
// everything from the artifacts and compares roots, so anyone can rerun it.
package snapshot

import (
	"fmt"
	"math/big"
	"path/filepath"
	"sort"

	"github.com/hearthchain/genesis/internal/bindings"
	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/chain/chains"
	"github.com/hearthchain/genesis/internal/credit"
	"github.com/hearthchain/genesis/internal/evidence"
	"github.com/hearthchain/genesis/internal/journal"
	"github.com/hearthchain/genesis/internal/layers"
	"github.com/hearthchain/genesis/internal/store"
)

// Entry is one snapshot line: a Hearth address and its total credit.
type Entry struct {
	Hearth      string `json:"hearth"`
	CreditMicro string `json:"creditMicro"`
}

// Snapshot is the aggregate result; Entries are sorted by Hearth address and
// the Merkle root is computed over "hearth:creditMicro" leaves in that order.
type Snapshot struct {
	Entries          []Entry  `json:"entries"`
	Root             string   `json:"merkleRoot"`
	TotalCreditMicro string   `json:"totalCreditMicro"`
	PendingSources   []string `json:"pendingSources,omitempty"`
	BlockedSources   []string `json:"blockedSources,omitempty"`
}

// burnRow is the subset of a burns.jsonl line the snapshot needs; the JSONL
// schema, not a Go package, is the contract between watcher and snapshot.
type burnRow struct {
	TxID   string `json:"txId"`
	Chain  string `json:"chain"`
	Source string `json:"source"`
	Amount uint64 `json:"amountBaseUnits"`
	Height uint64 `json:"height"`
	Status string `json:"status"`
}

// Build computes the snapshot and the evidence bundles from a data directory.
// journals maps each chain slug to its weekly price journal.
func Build(dataDir string, journals map[string]*journal.Journal, hearthScheme byte) (Snapshot, []evidence.Bundle, error) {
	confirmed, err := confirmedBurnsBySource(filepath.Join(dataDir, "burns.jsonl"))
	if err != nil {
		return Snapshot{}, nil, err
	}
	reg, err := bindings.Load(filepath.Join(dataDir, "bindings.jsonl"), hearthScheme)
	if err != nil {
		return Snapshot{}, nil, err
	}

	var (
		snap    Snapshot
		bundles []evidence.Bundle
		totals  = map[string]*big.Int{}
		total   = new(big.Int)
	)
	for _, source := range sortedKeys(confirmed) {
		srcBundles, credited, verdict, srcErr := creditSource(dataDir, journals, source, confirmed[source], reg)
		if srcErr != nil {
			return Snapshot{}, nil, srcErr
		}
		switch verdict {
		case sourceBlocked:
			snap.BlockedSources = append(snap.BlockedSources, source)
		case sourcePending:
			snap.PendingSources = append(snap.PendingSources, source)
		case sourceCredited:
			hearth := srcBundles[0].Hearth
			if totals[hearth] == nil {
				totals[hearth] = new(big.Int)
			}
			totals[hearth].Add(totals[hearth], credited)
			total.Add(total, credited)
		}
		bundles = append(bundles, srcBundles...)
	}

	for _, hearth := range sortedKeys(totals) {
		snap.Entries = append(snap.Entries, Entry{Hearth: hearth, CreditMicro: totals[hearth].String()})
	}
	leaves := make([][]byte, 0, len(snap.Entries))
	for _, e := range snap.Entries {
		leaves = append(leaves, []byte(e.Hearth+":"+e.CreditMicro))
	}
	snap.Root = MerkleRoot(leaves)
	snap.TotalCreditMicro = total.String()
	return snap, bundles, nil
}

const (
	sourceCredited = "credited"
	sourcePending  = "pending"
	sourceBlocked  = "blocked"
)

// creditSource prices all burns of one source address against its verified
// transfer history. Blocked histories yield no bundles; unbound sources yield
// bundles without a Hearth destination and stay out of the totals.
// chainRules resolves the per-chain pieces of the credit pipeline.
func chainRules(
	journals map[string]*journal.Journal, chainName string,
) (*journal.Journal, chains.DeltaFunc, uint64, error) {
	j, ok := journals[chainName]
	if !ok {
		return nil, nil, 0, fmt.Errorf("snapshot: no journal for chain %q", chainName)
	}
	deltasFor, err := chains.DeltasFor(chainName)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("snapshot: %w", err)
	}
	units, err := chains.BaseUnits(chainName)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("snapshot: %w", err)
	}
	return j, deltasFor, units, nil
}

func creditSource(
	dataDir string, journals map[string]*journal.Journal, source string, burns []burnRow, reg *bindings.Registry,
) ([]evidence.Bundle, *big.Int, string, error) {
	chainName := burns[0].Chain
	j, deltasFor, units, err := chainRules(journals, chainName)
	if err != nil {
		return nil, nil, "", err
	}
	transfersPath := filepath.Join(dataDir, "transfers", chainName, source+".jsonl")
	meta, txs, err := store.ReadTransfers(transfersPath)
	if err != nil || meta.Status != "ok" {
		return nil, nil, sourceBlocked, nil //nolint:nilerr // a missing or failed history blocks the source, not the snapshot
	}
	deltas, status := deltasFor(txs, source)
	if status.Kind != "ok" {
		return nil, nil, sourceBlocked, nil
	}
	deltas = chain.WithOpening(deltas, meta.OpeningBaseUnits, meta.OpeningAt)
	burnAmounts := make(map[string]uint64, len(burns))
	for _, b := range burns {
		burnAmounts[b.TxID] = b.Amount
	}
	_, consumed, err := layers.Build(deltas, burnAmounts)
	if err != nil {
		return nil, nil, sourceBlocked, nil //nolint:nilerr // an inconsistent layer profile blocks the source
	}
	sha, err := store.Sha256File(transfersPath)
	if err != nil {
		return nil, nil, "", err
	}
	hearth, bound := reg.HearthFor(source)

	credited := new(big.Int)
	bundles := make([]evidence.Bundle, 0, len(burns))
	for _, b := range burns {
		totalMic, perLayer, cErr := credit.Compute(consumed[b.TxID], j, units)
		if cErr != nil {
			return nil, nil, "", fmt.Errorf("snapshot: %s: %w", b.TxID, cErr)
		}
		bundle := evidence.Bundle{
			TxID:            b.TxID,
			Chain:           b.Chain,
			Source:          source,
			AmountBaseUnits: b.Amount,
			Height:          b.Height,
			Layers:          perLayer,
			CreditMicro:     totalMic.String(),
			ReferenceHeight: meta.ReferenceHeight,
			TransfersSha256: sha,
		}
		if bound {
			bundle.Hearth = hearth
		}
		bundles = append(bundles, bundle)
		credited.Add(credited, totalMic)
	}
	if !bound {
		return bundles, nil, sourcePending, nil
	}
	return bundles, credited, sourceCredited, nil
}

func confirmedBurnsBySource(path string) (map[string][]burnRow, error) {
	rows, err := store.ReadJSONL[burnRow](path)
	if err != nil {
		return nil, err
	}
	latest := make(map[string]burnRow, len(rows))
	for _, r := range rows {
		latest[r.TxID] = r
	}
	bySource := map[string][]burnRow{}
	for _, r := range latest {
		if r.Status == "confirmed" {
			bySource[r.Source] = append(bySource[r.Source], r)
		}
	}
	for _, burns := range bySource {
		sort.Slice(burns, func(i, j int) bool { return burns[i].TxID < burns[j].TxID })
	}
	return bySource, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
