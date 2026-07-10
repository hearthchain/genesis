package eos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// ErrHistoryTooLarge marks an account whose action history exceeds the cap;
// such accounts (exchanges, contracts) go to manual review instead of
// hammering the public index.
var ErrHistoryTooLarge = errors.New("eos: history exceeds the action cap; manual review")

// hyperionPageLimit is the instance-advertised get_actions row maximum.
const hyperionPageLimit = 1000

// transferFilter selects both native token contracts' transfer actions.
const transferFilter = contractLegacy + ":transfer," + contractVaulta + ":transfer"

// Hyperion is a client of a Hyperion v2 history API: the only public source
// of EOS account history (chain API nodes serve none).
type Hyperion struct {
	base      string
	http      *http.Client
	pageLimit int
}

// HyperionOption configures the client.
type HyperionOption func(*Hyperion)

// WithHyperionPageLimit overrides the page size (tests use small pages).
func WithHyperionPageLimit(n int) HyperionOption {
	return func(h *Hyperion) { h.pageLimit = n }
}

// NewHyperion wraps a Hyperion base URL.
func NewHyperion(base string, opts ...HyperionOption) *Hyperion {
	h := &Hyperion{base: base, http: &http.Client{Timeout: requestTimeout}, pageLimit: hyperionPageLimit}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// TransferActions lists every native-token transfer action involving the
// account, both directions, ascending. maxActions is the hard cap.
func (h *Hyperion) TransferActions(ctx context.Context, account string, maxActions int) ([]json.RawMessage, error) {
	return h.fetchAll(ctx, account, "", maxActions)
}

// TransfersTo lists the native-token transfers received by the account
// (the burn target), ascending. maxActions is the hard cap.
func (h *Hyperion) TransfersTo(ctx context.Context, account string, maxActions int) ([]json.RawMessage, error) {
	return h.fetchAll(ctx, account, account, maxActions)
}

func (h *Hyperion) fetchAll(ctx context.Context, account, to string, maxActions int) ([]json.RawMessage, error) {
	var out []json.RawMessage
	for skip := 0; ; skip += h.pageLimit {
		page, total, err := h.page(ctx, account, to, skip)
		if err != nil {
			return nil, err
		}
		if total > maxActions {
			return nil, fmt.Errorf("%w: %s has %d actions, cap %d", ErrHistoryTooLarge, account, total, maxActions)
		}
		out = append(out, page...)
		if len(page) < h.pageLimit || len(out) >= total {
			return out, nil
		}
	}
}

func (h *Hyperion) page(ctx context.Context, account, to string, skip int) ([]json.RawMessage, int, error) {
	query := url.Values{
		"account": {account},
		"filter":  {transferFilter},
		"sort":    {"asc"},
		"limit":   {strconv.Itoa(h.pageLimit)},
		"skip":    {strconv.Itoa(skip)},
	}
	if to != "" {
		query.Set("transfer.to", to)
	}
	endpoint := h.base + "/v2/history/get_actions?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, 0, fmt.Errorf("eos: hyperion: %w", err)
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("eos: hyperion: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("eos: hyperion: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("eos: hyperion: HTTP %d: %.200s", resp.StatusCode, raw)
	}
	var body struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Actions []json.RawMessage `json:"actions"`
	}
	if uErr := json.Unmarshal(raw, &body); uErr != nil {
		return nil, 0, fmt.Errorf("eos: hyperion: %w", uErr)
	}
	return body.Actions, body.Total.Value, nil
}
