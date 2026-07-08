// Package chains is the registry wiring chain slugs to their adapters and
// per-chain constants. Offline consumers (snapshot) use DeltasFor and
// BaseUnits without constructing node clients.
package chains

import (
	"encoding/json"
	"fmt"

	"github.com/hearthchain/burning-page/internal/chain"
	"github.com/hearthchain/burning-page/internal/chain/waves"
	"github.com/hearthchain/burning-page/internal/config"
)

// New builds the live adapter for the named chain from its config block.
func New(name string, cc config.ChainConfig) (chain.Adapter, error) {
	switch name {
	case Waves:
		return &waves.Adapter{
			Primary:     waves.NewClient(cc.Nodes.Primary),
			Secondary:   waves.NewClient(cc.Nodes.Secondary),
			BurnAddress: cc.BurnAddress,
		}, nil
	default:
		return nil, fmt.Errorf("chains: unknown chain %q", name)
	}
}

// NewFixture builds a fixture-backed adapter over a directory instead of
// live nodes (offline end-to-end mode).
func NewFixture(name, dir string, cc config.ChainConfig) (chain.Adapter, error) {
	switch name {
	case Waves:
		node := waves.NewFileNode(dir)
		return &waves.Adapter{Primary: node, Secondary: node, BurnAddress: cc.BurnAddress}, nil
	default:
		return nil, fmt.Errorf("chains: unknown chain %q", name)
	}
}

// DeltaFunc replays raw history rows into signed balance changes.
type DeltaFunc func(txs []json.RawMessage, addr string) ([]chain.Delta, chain.Status)

const (
	// Waves is the chain slug of the Waves mainnet adapter.
	Waves = "waves"
	// wavesBaseUnits is wavelets per WAVES (8 decimals).
	wavesBaseUnits = 100_000_000
)

// DeltasFor returns the pure delta-replay rule of the named chain.
func DeltasFor(name string) (DeltaFunc, error) {
	switch name {
	case Waves:
		return waves.Deltas, nil
	default:
		return nil, fmt.Errorf("chains: unknown chain %q", name)
	}
}

// BaseUnits returns how many base units make one whole coin on the named
// chain (wavelets per WAVES, 10^4 base units per A).
func BaseUnits(name string) (uint64, error) {
	switch name {
	case Waves:
		return wavesBaseUnits, nil
	default:
		return 0, fmt.Errorf("chains: unknown chain %q", name)
	}
}
