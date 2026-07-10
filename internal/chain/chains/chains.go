// Package chains is the registry wiring chain slugs to their adapters and
// per-chain constants. Offline consumers (snapshot) use DeltasFor and
// BaseUnits without constructing node clients.
package chains

import (
	"encoding/json"
	"fmt"

	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/chain/eos"
	"github.com/hearthchain/genesis/internal/chain/waves"
	"github.com/hearthchain/genesis/internal/config"
)

// New builds the live adapter for the named chain from its config block.
// hearthScheme feeds the chains that verify Hearth addresses on-adapter
// (the EOS memo bindings).
func New(name string, cc config.ChainConfig, hearthScheme byte) (chain.Adapter, error) {
	switch name {
	case Waves:
		return &waves.Adapter{
			Primary:     waves.NewClient(cc.Nodes.Primary),
			Secondary:   waves.NewClient(cc.Nodes.Secondary),
			BurnAddress: cc.BurnAddress,
		}, nil
	case Eos:
		if cc.HistoryAPI == "" {
			return nil, fmt.Errorf("chains: eos: historyAPI (the Hyperion base URL) is required")
		}
		return &eos.Adapter{
			API:          eos.NewClient(cc.Nodes.Primary),
			Index:        eos.NewHyperion(cc.HistoryAPI),
			Secondary:    eos.NewGreymass(cc.Nodes.Secondary),
			BurnAccount:  cc.BurnAddress,
			HearthScheme: hearthScheme,
		}, nil
	default:
		return nil, fmt.Errorf("chains: unknown chain %q", name)
	}
}

// NewFixture builds a fixture-backed adapter over a directory instead of
// live nodes (offline end-to-end mode).
func NewFixture(name, dir string, cc config.ChainConfig, hearthScheme byte) (chain.Adapter, error) {
	switch name {
	case Waves:
		node := waves.NewFileNode(dir)
		return &waves.Adapter{Primary: node, Secondary: node, BurnAddress: cc.BurnAddress}, nil
	case Eos:
		source := eos.NewFixtureSource(dir)
		return &eos.Adapter{
			API:          source,
			Index:        source,
			Secondary:    source,
			BurnAccount:  cc.BurnAddress,
			HearthScheme: hearthScheme,
		}, nil
	default:
		return nil, fmt.Errorf("chains: no fixture adapter for chain %q", name)
	}
}

// DeltaFunc replays raw history rows into signed balance changes.
type DeltaFunc func(txs []json.RawMessage, addr string) ([]chain.Delta, chain.Status)

const (
	// Waves is the chain slug of the Waves mainnet adapter.
	Waves = "waves"
	// Eos is the chain slug of the EOS/Vaulta mainnet adapter.
	Eos = eos.Chain
	// wavesBaseUnits is wavelets per WAVES (8 decimals).
	wavesBaseUnits = 100_000_000
)

// DeltasFor returns the pure delta-replay rule of the named chain.
func DeltasFor(name string) (DeltaFunc, error) {
	switch name {
	case Waves:
		return waves.Deltas, nil
	case Eos:
		return eos.Deltas, nil
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
	case Eos:
		return eos.BaseUnits, nil
	default:
		return 0, fmt.Errorf("chains: unknown chain %q", name)
	}
}
