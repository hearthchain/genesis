package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TransferMeta heads a transfers artifact: the safety-invariant verdict for
// one source address. Status "ok" means the recomputed balance matched the
// node exactly; anything else blocks the address's credits to manual review.
type TransferMeta struct {
	Address         string    `json:"address"`
	Chain           string    `json:"chain"`
	FetchedAt       time.Time `json:"fetchedAt"`
	ReferenceHeight uint64    `json:"referenceHeight"`
	NodeBalance     uint64    `json:"nodeBalanceBaseUnits"`
	Recomputed      int64     `json:"recomputedBaseUnits"`
	// OpeningBaseUnits and OpeningAt describe the synthetic opening layer of
	// a truncated public history; zero means complete from genesis.
	OpeningBaseUnits uint64    `json:"openingBaseUnits,omitempty"`
	OpeningAt        time.Time `json:"openingAt,omitzero"`
	Status           string    `json:"status"`
	Reason           string    `json:"reason,omitempty"`
}

type metaLine struct {
	Meta *TransferMeta `json:"meta"`
}

// WriteTransfers writes a transfers artifact: one meta line, then the verbatim
// node transaction JSON, ascending. The write replaces any previous artifact.
func WriteTransfers(path string, meta TransferMeta, txs []json.RawMessage) error {
	if mkErr := os.MkdirAll(filepath.Dir(path), dirPerm); mkErr != nil {
		return fmt.Errorf("store: %w", mkErr)
	}
	head, err := json.Marshal(metaLine{Meta: &meta})
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	var out bytes.Buffer
	out.Write(head)
	out.WriteByte('\n')
	for _, tx := range txs {
		out.Write(tx)
		out.WriteByte('\n')
	}
	if wErr := os.WriteFile(path, out.Bytes(), filePerm); wErr != nil {
		return fmt.Errorf("store: %w", wErr)
	}
	return nil
}

// ReadTransfers reads a transfers artifact back into its meta and raw txs.
func ReadTransfers(path string) (TransferMeta, []json.RawMessage, error) {
	lines, err := ReadJSONL[json.RawMessage](path)
	if err != nil {
		return TransferMeta{}, nil, err
	}
	if len(lines) == 0 {
		return TransferMeta{}, nil, fmt.Errorf("store: %s: empty transfers artifact", path)
	}
	var head metaLine
	if uErr := json.Unmarshal(lines[0], &head); uErr != nil || head.Meta == nil {
		return TransferMeta{}, nil, fmt.Errorf("store: %s: first line is not a meta line: %w", path, uErr)
	}
	return *head.Meta, lines[1:], nil
}
