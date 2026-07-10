// Package evidence defines the per-credit proof bundle: everything a third
// party needs to recompute one credit from public data (the burn transaction,
// the priced layers, the invariant reference and the transfers-artifact
// checksum). Serialization is deterministic; the bundle's sha256 is its id in
// the published artifact set.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/hearthchain/burning-page/internal/credit"
)

// Bundle is one published credit proof.
type Bundle struct {
	TxID            string               `json:"txId"`
	Chain           string               `json:"chain"`
	Source          string               `json:"source"`
	Hearth          string               `json:"hearth,omitempty"`
	AmountBaseUnits uint64               `json:"amountBaseUnits"`
	Height          uint64               `json:"height"`
	Layers          []credit.LayerCredit `json:"layers"`
	CreditMicro     string               `json:"creditMicro"`
	ReferenceHeight uint64               `json:"referenceHeight"`
	TransfersSha256 string               `json:"transfersSha256"`
}

// Marshal serializes the bundle deterministically (fixed struct field order).
func (b Bundle) Marshal() ([]byte, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("evidence: %w", err)
	}
	return raw, nil
}

// Sha256 returns the hex digest of the deterministic serialization.
func (b Bundle) Sha256() (string, error) {
	raw, err := b.Marshal()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
