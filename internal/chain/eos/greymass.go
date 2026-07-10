package eos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Greymass is a client of the legacy v1 history get_transaction endpoint the
// Greymass nodes still answer: the independent second source every burn and
// binding is cross-checked against before it counts.
type Greymass struct {
	base string
	http *http.Client
}

// NewGreymass wraps a v1-history-capable node base URL.
func NewGreymass(base string) *Greymass {
	return &Greymass{base: base, http: &http.Client{Timeout: requestTimeout}}
}

// TransferTrace is one canonical native-token transfer execution inside a
// transaction (the contract's own trace, not the receiver notifications).
type TransferTrace struct {
	Contract string
	From     string
	To       string
	Quantity string
	Memo     string
}

// TransactionInfo is the cross-check projection of a v1 get_transaction
// response.
type TransactionInfo struct {
	ID           string
	BlockNum     uint64
	Irreversible bool
	Transfers    []TransferTrace
}

// GetTransaction fetches a transaction from the secondary source.
func (g *Greymass) GetTransaction(ctx context.Context, id string) (TransactionInfo, error) {
	payload, err := json.Marshal(map[string]string{"id": id})
	if err != nil {
		return TransactionInfo{}, fmt.Errorf("eos: greymass: %w", err)
	}
	endpoint := g.base + "/v1/history/get_transaction"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return TransactionInfo{}, fmt.Errorf("eos: greymass: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.http.Do(req)
	if err != nil {
		return TransactionInfo{}, fmt.Errorf("eos: greymass: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return TransactionInfo{}, fmt.Errorf("eos: greymass: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return TransactionInfo{}, fmt.Errorf("eos: greymass: HTTP %d: %.200s", resp.StatusCode, raw)
	}
	return decodeTransaction(raw)
}

func decodeTransaction(raw []byte) (TransactionInfo, error) {
	var body struct {
		ID           string `json:"id"`
		BlockNum     uint64 `json:"block_num"`
		Irreversible bool   `json:"irreversible"`
		Traces       []struct {
			Receiver string `json:"receiver"`
			Act      struct {
				Account string `json:"account"`
				Name    string `json:"name"`
				Data    struct {
					From     string `json:"from"`
					To       string `json:"to"`
					Quantity string `json:"quantity"`
					Memo     string `json:"memo"`
				} `json:"data"`
			} `json:"act"`
		} `json:"traces"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return TransactionInfo{}, fmt.Errorf("eos: greymass: %w", err)
	}
	info := TransactionInfo{ID: body.ID, BlockNum: body.BlockNum, Irreversible: body.Irreversible}
	for _, trace := range body.Traces {
		native := trace.Act.Account == contractLegacy || trace.Act.Account == contractVaulta
		if trace.Receiver != trace.Act.Account || trace.Act.Name != "transfer" || !native {
			continue
		}
		info.Transfers = append(info.Transfers, TransferTrace{
			Contract: trace.Act.Account,
			From:     trace.Act.Data.From,
			To:       trace.Act.Data.To,
			Quantity: trace.Act.Data.Quantity,
			Memo:     trace.Act.Data.Memo,
		})
	}
	return info, nil
}
