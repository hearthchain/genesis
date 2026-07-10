package eos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FixtureSource replays a fixture directory instead of the three live
// sources, letting the whole EOS pipeline run end-to-end offline:
//
//	info.json                 {"last_irreversible_block_num": N}
//	balances.json             {"<account>": combinedBaseUnits}
//	created.json              {"<account>": "2018-06-09T00:00:00.000"}
//	actions/<account>.json    [Hyperion get_actions rows, ascending]
//	transactions/<trx>.json   v1 get_transaction shape (id, irreversible, traces)
type FixtureSource struct {
	dir string
}

// NewFixtureSource opens a fixture directory.
func NewFixtureSource(dir string) *FixtureSource { return &FixtureSource{dir: dir} }

func (f *FixtureSource) read(parts ...string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(append([]string{f.dir}, parts...)...))
	if err != nil {
		return nil, fmt.Errorf("eos fixture: %w", err)
	}
	return raw, nil
}

// LastIrreversibleBlock reads info.json.
func (f *FixtureSource) LastIrreversibleBlock(context.Context) (uint64, error) {
	raw, err := f.read("info.json")
	if err != nil {
		return 0, err
	}
	var info struct {
		LastIrreversibleBlockNum uint64 `json:"last_irreversible_block_num"`
	}
	if uErr := json.Unmarshal(raw, &info); uErr != nil {
		return 0, fmt.Errorf("eos fixture: %w", uErr)
	}
	return info.LastIrreversibleBlockNum, nil
}

// CombinedBalance reads the account's entry in balances.json.
func (f *FixtureSource) CombinedBalance(_ context.Context, account string) (uint64, error) {
	raw, err := f.read("balances.json")
	if err != nil {
		return 0, err
	}
	balances := map[string]uint64{}
	if uErr := json.Unmarshal(raw, &balances); uErr != nil {
		return 0, fmt.Errorf("eos fixture: %w", uErr)
	}
	return balances[account], nil
}

// AccountCreated reads the account's entry in created.json.
func (f *FixtureSource) AccountCreated(_ context.Context, account string) (time.Time, error) {
	raw, err := f.read("created.json")
	if err != nil {
		return time.Time{}, err
	}
	created := map[string]string{}
	if uErr := json.Unmarshal(raw, &created); uErr != nil {
		return time.Time{}, fmt.Errorf("eos fixture: %w", uErr)
	}
	value, ok := created[account]
	if !ok {
		return time.Time{}, fmt.Errorf("eos fixture: no created date for %s", account)
	}
	return rowTime(value)
}

// TransferActions reads actions/<account>.json; a missing file is an empty
// history.
func (f *FixtureSource) TransferActions(_ context.Context, account string, _ int) ([]json.RawMessage, error) {
	raw, err := f.read("actions", account+".json")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []json.RawMessage
	if uErr := json.Unmarshal(raw, &rows); uErr != nil {
		return nil, fmt.Errorf("eos fixture: %s: %w", account, uErr)
	}
	return rows, nil
}

// TransfersTo serves the same per-account rows: the fixture author writes
// the burn account's file as its received transfers.
func (f *FixtureSource) TransfersTo(ctx context.Context, account string, maxActions int) ([]json.RawMessage, error) {
	return f.TransferActions(ctx, account, maxActions)
}

// GetTransaction reads transactions/<id>.json in v1 get_transaction shape.
func (f *FixtureSource) GetTransaction(_ context.Context, id string) (TransactionInfo, error) {
	raw, err := f.read("transactions", id+".json")
	if err != nil {
		return TransactionInfo{}, err
	}
	return decodeTransaction(raw)
}
