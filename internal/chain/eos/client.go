package eos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxResponseBytes bounds any single API response read.
const maxResponseBytes = 16 << 20

// requestTimeout bounds any single API request.
const requestTimeout = 30 * time.Second

// Client is a minimal wrapper over an Antelope chain API node (get_info,
// get_currency_balance, get_account). History does not live here: Antelope
// chain APIs have none; see Hyperion.
type Client struct {
	base string
	http *http.Client
}

// NewClient wraps a chain API base URL.
func NewClient(base string) *Client {
	return &Client{base: base, http: &http.Client{Timeout: requestTimeout}}
}

// post performs one chain-API POST with a JSON body and decodes the response.
func (c *Client) post(ctx context.Context, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("eos: %s: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("eos: %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("eos: %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("eos: %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("eos: %s: HTTP %d: %.200s", path, resp.StatusCode, raw)
	}
	if uErr := json.Unmarshal(raw, out); uErr != nil {
		return fmt.Errorf("eos: %s: %w", path, uErr)
	}
	return nil
}

// LastIrreversibleBlock returns the finalized tip: under Savanna consensus
// the LIB trails the head by about one second and is the finality anchor.
func (c *Client) LastIrreversibleBlock(ctx context.Context) (uint64, error) {
	var info struct {
		LastIrreversibleBlockNum uint64 `json:"last_irreversible_block_num"`
	}
	if err := c.post(ctx, "/v1/chain/get_info", struct{}{}, &info); err != nil {
		return 0, err
	}
	if info.LastIrreversibleBlockNum == 0 {
		return 0, fmt.Errorf("eos: get_info returned no last irreversible block")
	}
	return info.LastIrreversibleBlockNum, nil
}

// CombinedBalance sums the liquid A and legacy EOS balances of an account in
// base units; the two tokens are 1:1 fungible via the on-chain swap.
func (c *Client) CombinedBalance(ctx context.Context, account string) (uint64, error) {
	var total uint64
	for _, code := range []string{contractVaulta, contractLegacy} {
		body := map[string]string{"code": code, "account": account}
		var balances []string
		if err := c.post(ctx, "/v1/chain/get_currency_balance", body, &balances); err != nil {
			return 0, err
		}
		for _, quantity := range balances {
			units, err := ParseQuantity(quantity)
			if err != nil {
				return 0, err
			}
			total += units
		}
	}
	return total, nil
}

// AccountCreated returns the account's on-chain creation time.
func (c *Client) AccountCreated(ctx context.Context, account string) (time.Time, error) {
	var out struct {
		Created string `json:"created"`
	}
	if err := c.post(ctx, "/v1/chain/get_account", map[string]string{"account_name": account}, &out); err != nil {
		return time.Time{}, err
	}
	return rowTime(out.Created)
}
