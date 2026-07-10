package waves

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FileNode is a Node backed by a fixture directory instead of a live node:
//
//	height.json          {"height": N}
//	balances.json        {"<address>": wavelets}
//	history/<addr>.json  [[tx, ...]] in node page shape
//
// It lets the whole pipeline run end-to-end with no network and no real burns.
type FileNode struct {
	dir string
}

// NewFileNode opens a fixture directory.
func NewFileNode(dir string) *FileNode { return &FileNode{dir: dir} }

// AllTransactions reads history/<addr>.json; a missing file is an empty history.
func (f *FileNode) AllTransactions(_ context.Context, addr string) ([]json.RawMessage, error) {
	raw, err := os.ReadFile(filepath.Join(f.dir, "history", addr+".json")) //nolint:gosec // fixture dir is an operator flag
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("filenode: %w", err)
	}
	var outer [][]json.RawMessage
	if uErr := json.Unmarshal(raw, &outer); uErr != nil || len(outer) != 1 {
		return nil, fmt.Errorf("filenode: %s: not a node page: %w", addr, uErr)
	}
	return outer[0], nil
}

// Height reads height.json.
func (f *FileNode) Height(context.Context) (uint64, error) {
	raw, err := os.ReadFile(filepath.Join(f.dir, "height.json"))
	if err != nil {
		return 0, fmt.Errorf("filenode: %w", err)
	}
	var h struct {
		Height uint64 `json:"height"`
	}
	if uErr := json.Unmarshal(raw, &h); uErr != nil {
		return 0, fmt.Errorf("filenode: %w", uErr)
	}
	return h.Height, nil
}

// BalanceAfterConfirmations reads the address's entry in balances.json.
func (f *FileNode) BalanceAfterConfirmations(_ context.Context, addr string, _ uint64) (uint64, error) {
	raw, err := os.ReadFile(filepath.Join(f.dir, "balances.json"))
	if err != nil {
		return 0, fmt.Errorf("filenode: %w", err)
	}
	balances := map[string]uint64{}
	if uErr := json.Unmarshal(raw, &balances); uErr != nil {
		return 0, fmt.Errorf("filenode: %w", uErr)
	}
	return balances[addr], nil
}

// TransactionInfo scans every fixture history for the transaction id.
func (f *FileNode) TransactionInfo(ctx context.Context, id string) (json.RawMessage, error) {
	entries, err := os.ReadDir(filepath.Join(f.dir, "history"))
	if err != nil {
		return nil, fmt.Errorf("filenode: %w", err)
	}
	for _, e := range entries {
		addr, ok := cutJSONSuffix(e.Name())
		if !ok {
			continue
		}
		txs, txErr := f.AllTransactions(ctx, addr)
		if txErr != nil {
			return nil, txErr
		}
		for _, tx := range txs {
			var peek struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(tx, &peek) == nil && peek.ID == id {
				return tx, nil
			}
		}
	}
	return nil, fmt.Errorf("filenode: transaction %s not in fixtures", id)
}

func cutJSONSuffix(name string) (string, bool) {
	const suffix = ".json"
	if len(name) <= len(suffix) || name[len(name)-len(suffix):] != suffix {
		return "", false
	}
	return name[:len(name)-len(suffix)], true
}
