package waves

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hearthchain/burning-page/internal/chain"
)

// Transaction type ids of the Waves protocol that the MVP interprets.
const (
	txGenesis      = 1
	txPayment      = 2
	txTransfer     = 4
	txMassTransfer = 11
)

// burnTx is the minimal projection of a node transaction JSON the detector
// and delta rules need; Raw keeps the verbatim bytes for the artifact.
type burnTx struct {
	Type              int     `json:"type"`
	ID                string  `json:"id"`
	Sender            string  `json:"sender"`
	Recipient         string  `json:"recipient"`
	AssetID           *string `json:"assetId"`
	Amount            uint64  `json:"amount"`
	Timestamp         int64   `json:"timestamp"`
	Height            uint64  `json:"height"`
	ApplicationStatus string  `json:"applicationStatus"`
	Transfers         []struct {
		Recipient string `json:"recipient"`
		Amount    uint64 `json:"amount"`
	} `json:"transfers"`
}

// DetectBurns filters an address history down to the burns inside the window:
// WAVES transfers (plain or mass) to the burn address with height in
// [window.Start, window.End]. Maturity (confirmation depth) is the watcher's
// call, not the detector's: fresh burns surface immediately as pending.
// Attachments are tolerated; the funds are destroyed regardless of extra data.
func DetectBurns(txs []json.RawMessage, burnAddr string, window chain.Window) ([]chain.Burn, error) {
	maxHeight := window.End
	var burns []chain.Burn
	for _, raw := range txs {
		var tx burnTx
		if err := json.Unmarshal(raw, &tx); err != nil {
			return nil, fmt.Errorf("waves: burn detection: %w", err)
		}
		if !tx.confirmedWavesMove() || tx.Height < window.Start || tx.Height > maxHeight {
			continue
		}
		amount := tx.amountTo(burnAddr)
		if amount == 0 {
			continue
		}
		burns = append(burns, chain.Burn{
			TxID:      tx.ID,
			Chain:     "waves",
			Source:    tx.Sender,
			Amount:    amount,
			Height:    tx.Height,
			Timestamp: time.UnixMilli(tx.Timestamp).UTC(),
			Raw:       raw,
		})
	}
	return burns, nil
}

// confirmedWavesMove reports whether the tx is a succeeded WAVES value
// transfer of a type the burn detector interprets.
func (tx *burnTx) confirmedWavesMove() bool {
	if tx.Type != txTransfer && tx.Type != txMassTransfer {
		return false
	}
	if tx.AssetID != nil {
		return false
	}
	return tx.ApplicationStatus == "" || tx.ApplicationStatus == "succeeded"
}

// amountTo sums the WAVES the transaction moves to the given address.
func (tx *burnTx) amountTo(addr string) uint64 {
	if tx.Type == txTransfer {
		if tx.Recipient == addr {
			return tx.Amount
		}
		return 0
	}
	var sum uint64
	for _, entry := range tx.Transfers {
		if entry.Recipient == addr {
			sum += entry.Amount
		}
	}
	return sum
}
