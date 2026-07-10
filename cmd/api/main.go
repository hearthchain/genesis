// Command api serves the burn backend's read/submit endpoints.
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/hearthchain/genesis/internal/api"
	"github.com/hearthchain/genesis/internal/bindings"
	"github.com/hearthchain/genesis/internal/chain"
	"github.com/hearthchain/genesis/internal/chain/chains"
	"github.com/hearthchain/genesis/internal/config"
	"github.com/hearthchain/genesis/internal/journal"
)

const readHeaderTimeout = 10 * time.Second

func main() { os.Exit(run()) }

func run() int {
	configPath := flag.String("config", "config.json", "path to the shared config")
	fixture := flag.String("fixture", "", "fixture directory replacing the node (offline end-to-end mode)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("config", "err", err)
		return 1
	}
	reg, err := bindings.Load(cfg.DataDir+"/bindings.jsonl", cfg.HearthSchemeByte())
	if err != nil {
		slog.Error("bindings", "err", err)
		return 1
	}
	adapters := map[string]chain.Adapter{}
	journals := map[string]*journal.Journal{}
	for name, cc := range cfg.Chains {
		j, jErr := journal.Load(cc.JournalCSV)
		if jErr != nil {
			slog.Error("journal", "chain", name, "err", jErr)
			return 1
		}
		journals[name] = j
		var adapter chain.Adapter
		var aErr error
		if *fixture != "" {
			adapter, aErr = chains.NewFixture(name, *fixture, cc, cfg.HearthSchemeByte())
		} else {
			adapter, aErr = chains.New(name, cc, cfg.HearthSchemeByte())
		}
		if aErr != nil {
			slog.Error("adapter", "chain", name, "err", aErr)
			return 1
		}
		adapters[name] = adapter
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.New(adapters, journals, reg, cfg).Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	slog.Info("api listening", "addr", cfg.ListenAddr)
	if serveErr := srv.ListenAndServe(); serveErr != nil {
		slog.Error("serve", "err", serveErr)
		return 1
	}
	return 0
}
