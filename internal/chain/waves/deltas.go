package waves

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/hearthchain/burning-page/internal/chain"
)

// Deltas reconstructs the signed WAVES balance changes of addr from its full
// transaction history. Supported types: Genesis, Payment, Transfer and
// MassTransfer; anything else yields an unsupported status.
func Deltas(txs []json.RawMessage, addr string) ([]chain.Delta, chain.Status) {
	deltas := make([]chain.Delta, 0, len(txs))
	for _, raw := range txs {
		var tx deltaTx
		if err := json.Unmarshal(raw, &tx); err != nil {
			return nil, chain.Status{Kind: chain.StatusUnsupported, Reason: fmt.Sprintf("undecodable transaction: %v", err)}
		}
		amount, ok := tx.deltaFor(addr)
		if !ok {
			return nil, chain.Status{Kind: chain.StatusUnsupported, Reason: fmt.Sprintf("transaction type %d (id %s)", tx.Type, tx.ID)}
		}
		deltas = append(deltas, chain.Delta{
			TxID:      tx.ID,
			Height:    tx.Height,
			Timestamp: time.UnixMilli(tx.Timestamp).UTC(),
			Amount:    amount,
		})
	}
	sort.SliceStable(deltas, func(i, j int) bool { return deltas[i].Height < deltas[j].Height })
	return deltas, chain.Status{Kind: chain.StatusOK}
}

// deltaTx is the projection of a node transaction the delta rules read.
type deltaTx struct {
	Type       int     `json:"type"`
	ID         string  `json:"id"`
	Sender     string  `json:"sender"`
	Recipient  string  `json:"recipient"`
	AssetID    *string `json:"assetId"`
	FeeAssetID *string `json:"feeAssetId"`
	Amount     int64   `json:"amount"`
	Fee        int64   `json:"fee"`
	Timestamp  int64   `json:"timestamp"`
	Height     uint64  `json:"height"`
	Transfers  []struct {
		Recipient string `json:"recipient"`
		Amount    int64  `json:"amount"`
	} `json:"transfers"`
}

// deltaFor returns the signed WAVES change this tx causes for addr, or
// ok=false when the transaction type is outside the supported set.
func (tx *deltaTx) deltaFor(addr string) (int64, bool) {
	switch tx.Type {
	case txGenesis:
		return tx.incomingOnly(addr), true
	case txPayment:
		return tx.paymentDelta(addr), true
	case txTransfer:
		return tx.transferDelta(addr), true
	case txMassTransfer:
		return tx.massTransferDelta(addr), true
	default:
		return 0, false
	}
}

func (tx *deltaTx) incomingOnly(addr string) int64 {
	if tx.Recipient == addr {
		return tx.Amount
	}
	return 0
}

func (tx *deltaTx) paymentDelta(addr string) int64 {
	var delta int64
	if tx.Sender == addr {
		delta -= tx.Amount + tx.Fee
	}
	if tx.Recipient == addr {
		delta += tx.Amount
	}
	return delta
}

func (tx *deltaTx) transferDelta(addr string) int64 {
	var delta int64
	if tx.Sender == addr {
		if tx.FeeAssetID == nil {
			delta -= tx.Fee
		}
		if tx.AssetID == nil {
			delta -= tx.Amount
		}
	}
	if tx.Recipient == addr && tx.AssetID == nil {
		delta += tx.Amount
	}
	return delta
}

func (tx *deltaTx) massTransferDelta(addr string) int64 {
	var delta int64
	if tx.Sender == addr {
		delta -= tx.Fee // mass-transfer fees are always paid in WAVES
		if tx.AssetID == nil {
			for _, entry := range tx.Transfers {
				delta -= entry.Amount
			}
		}
	}
	if tx.AssetID == nil {
		for _, entry := range tx.Transfers {
			if entry.Recipient == addr {
				delta += entry.Amount
			}
		}
	}
	return delta
}
