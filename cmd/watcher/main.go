// Command watcher polls Waves mainnet for burns and writes the artifacts.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hearthchain/burning-page/internal/bindings"
	"github.com/hearthchain/burning-page/internal/chain"
	"github.com/hearthchain/burning-page/internal/chain/chains"
	"github.com/hearthchain/burning-page/internal/config"
	"github.com/hearthchain/burning-page/internal/watcher"
)

const pollInterval = 60 * time.Second // ~Waves block time; polling faster buys nothing

func main() { os.Exit(run()) }

func run() int {
	configPath := flag.String("config", "config.json", "path to the shared config")
	chainName := flag.String("chain", "waves", "which configured chain to watch (one process per chain)")
	once := flag.Bool("once", false, "run a single poll and exit")
	fixture := flag.String("fixture", "", "fixture directory replacing both nodes (offline end-to-end mode)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("config", "err", err)
		return 1
	}
	cc, ok := cfg.Chains[*chainName]
	if !ok {
		slog.Error("config", "err", "chain not configured", "chain", *chainName)
		return 1
	}
	var adapter chain.Adapter
	if *fixture != "" {
		adapter, err = chains.NewFixture(*chainName, *fixture, cc, cfg.HearthSchemeByte())
	} else {
		adapter, err = chains.New(*chainName, cc, cfg.HearthSchemeByte())
	}
	if err != nil {
		slog.Error("adapter", "err", err)
		return 1
	}
	reg, err := bindings.Load(cfg.DataDir+"/bindings.jsonl", cfg.HearthSchemeByte())
	if err != nil {
		slog.Error("bindings", "err", err)
		return 1
	}
	w := &watcher.Watcher{Adapter: adapter, ChainCfg: cc, DataDir: cfg.DataDir, Registry: reg}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	for {
		if pollErr := w.Poll(ctx); pollErr != nil {
			slog.Error("poll", "err", pollErr)
			if *once {
				return 1
			}
		}
		if *once {
			return 0
		}
		select {
		case <-ctx.Done():
			return 0
		case <-time.After(pollInterval):
		}
	}
}
