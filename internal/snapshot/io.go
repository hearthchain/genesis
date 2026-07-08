package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hearthchain/burning-page/internal/evidence"
	"github.com/hearthchain/burning-page/internal/journal"
)

const (
	filePerm = 0o600
	dirPerm  = 0o750
)

// Write persists snapshot.json plus one evidence/<txId>.json per bundle.
// Output is deterministic: rerunning Write on the same inputs is byte-identical.
func Write(dataDir string, snap Snapshot, bundles []evidence.Bundle) error {
	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	if wErr := os.WriteFile(filepath.Join(dataDir, "snapshot.json"), append(raw, '\n'), filePerm); wErr != nil {
		return fmt.Errorf("snapshot: %w", wErr)
	}
	evidenceDir := filepath.Join(dataDir, "evidence")
	if mkErr := os.MkdirAll(evidenceDir, dirPerm); mkErr != nil {
		return fmt.Errorf("snapshot: %w", mkErr)
	}
	for _, b := range bundles {
		body, mErr := b.Marshal()
		if mErr != nil {
			return mErr
		}
		path := filepath.Join(evidenceDir, b.TxID+".json")
		if wErr := os.WriteFile(path, append(body, '\n'), filePerm); wErr != nil {
			return fmt.Errorf("snapshot: %w", wErr)
		}
	}
	return nil
}

// Verify rebuilds the snapshot from the artifacts and compares Merkle roots
// with the stored snapshot.json: the anyone-can-recompute check.
func Verify(dataDir string, journals map[string]*journal.Journal, hearthScheme byte) error {
	raw, err := os.ReadFile(filepath.Join(dataDir, "snapshot.json")) //nolint:gosec // dataDir is an operator flag
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	var stored Snapshot
	if uErr := json.Unmarshal(raw, &stored); uErr != nil {
		return fmt.Errorf("snapshot: stored snapshot.json: %w", uErr)
	}
	rebuilt, _, err := Build(dataDir, journals, hearthScheme)
	if err != nil {
		return err
	}
	if rebuilt.Root != stored.Root {
		return fmt.Errorf("snapshot: root mismatch: rebuilt %s, stored %s", rebuilt.Root, stored.Root)
	}
	return nil
}
