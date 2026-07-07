package waves

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultPageLimit = 1000 // documented maximum of /transactions/address
	requestTimeout   = 30 * time.Second
)

// Client is a thin REST client for a public Waves node. It goes through the
// raw HTTP API because the gowaves client lacks the after-cursor pagination
// that full address histories need; responses stay as verbatim JSON so the
// recorded artifacts are byte-reproducible against the node.
type Client struct {
	base      string
	hc        *http.Client
	pageLimit int
}

// Option configures a Client.
type Option func(*Client)

// WithPageLimit overrides the page size of AllTransactions (tests use small
// recorded pages; production uses the node maximum).
func WithPageLimit(n int) Option {
	return func(c *Client) { c.pageLimit = n }
}

// NewClient builds a client for the node at base, e.g. "https://nodes.wavesnodes.com".
func NewClient(base string, opts ...Option) *Client {
	c := &Client{
		base:      base,
		hc:        &http.Client{Timeout: requestTimeout},
		pageLimit: defaultPageLimit,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// AllTransactions fetches the complete transaction history of an address,
// newest first, following the after cursor until a short page.
func (c *Client) AllTransactions(ctx context.Context, addr string) ([]json.RawMessage, error) {
	var all []json.RawMessage
	after := ""
	for {
		page, err := c.transactionsPage(ctx, addr, after)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < c.pageLimit {
			return all, nil
		}
		var last struct {
			ID string `json:"id"`
		}
		if cursorErr := json.Unmarshal(page[len(page)-1], &last); cursorErr != nil || last.ID == "" {
			return nil, fmt.Errorf("waves: page cursor: cannot read last tx id: %w", cursorErr)
		}
		after = last.ID
	}
}

func (c *Client) transactionsPage(ctx context.Context, addr, after string) ([]json.RawMessage, error) {
	path := fmt.Sprintf("/transactions/address/%s/limit/%d", url.PathEscape(addr), c.pageLimit)
	if after != "" {
		path += "?after=" + url.QueryEscape(after)
	}
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	var outer []json.RawMessage
	if outerErr := json.Unmarshal(body, &outer); outerErr != nil {
		return nil, fmt.Errorf("waves: %s: %w", path, outerErr)
	}
	if len(outer) != 1 {
		return nil, fmt.Errorf("waves: %s: expected a single-element outer array, got %d", path, len(outer))
	}
	var txs []json.RawMessage
	if txsErr := json.Unmarshal(outer[0], &txs); txsErr != nil {
		return nil, fmt.Errorf("waves: %s: %w", path, txsErr)
	}
	return txs, nil
}

// Height returns the node's current blockchain height.
func (c *Client) Height(ctx context.Context) (uint64, error) {
	body, err := c.get(ctx, "/blocks/height")
	if err != nil {
		return 0, err
	}
	var h struct {
		Height uint64 `json:"height"`
	}
	if hErr := json.Unmarshal(body, &h); hErr != nil {
		return 0, fmt.Errorf("waves: /blocks/height: %w", hErr)
	}
	return h.Height, nil
}

// BalanceAfterConfirmations returns the WAVES balance (wavelets) as of the
// given confirmation depth: the balance at height tip-confirmations.
func (c *Client) BalanceAfterConfirmations(ctx context.Context, addr string, confirmations uint64) (uint64, error) {
	path := fmt.Sprintf("/addresses/balance/%s/%d", url.PathEscape(addr), confirmations)
	body, err := c.get(ctx, path)
	if err != nil {
		return 0, err
	}
	var b struct {
		Balance uint64 `json:"balance"`
	}
	if bErr := json.Unmarshal(body, &b); bErr != nil {
		return 0, fmt.Errorf("waves: %s: %w", path, bErr)
	}
	return b.Balance, nil
}

// TransactionInfo fetches one confirmed transaction (with height) by id.
func (c *Client) TransactionInfo(ctx context.Context, id string) (json.RawMessage, error) {
	return c.get(ctx, "/transactions/info/"+url.PathEscape(id))
}

// get performs one GET with a single jittered retry: public nodes rate-limit,
// and one retry rides out a transient 429 without hiding real outages.
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	body, err := c.getOnce(ctx, path)
	if err == nil {
		return body, nil
	}
	const jitterBaseMs, jitterSpreadMs = 500, 1500
	jitter := time.Duration(jitterBaseMs+rand.IntN(jitterSpreadMs)) * time.Millisecond //nolint:gosec // jitter needs no crypto randomness
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(jitter):
	}
	return c.getOnce(ctx, path)
}

func (c *Client) getOnce(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("waves: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("waves: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close on a read-only request
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("waves: GET %s: status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("waves: GET %s: %w", path, err)
	}
	return body, nil
}
