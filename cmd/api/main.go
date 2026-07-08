// Command api serves the burn backend's read/submit endpoints.
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/hearthchain/burning-page/internal/api"
	"github.com/hearthchain/burning-page/internal/bindings"
	"github.com/hearthchain/burning-page/internal/chain/waves"
	"github.com/hearthchain/burning-page/internal/config"
	"github.com/hearthchain/burning-page/internal/journal"
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
	wavesCfg, ok := cfg.Chains["waves"]
	if !ok {
		slog.Error("config", "err", "no waves chain block")
		return 1
	}
	j, err := journal.Load(wavesCfg.JournalCSV)
	if err != nil {
		slog.Error("journal", "err", err)
		return 1
	}
	reg, err := bindings.Load(cfg.DataDir+"/bindings.jsonl", cfg.HearthSchemeByte())
	if err != nil {
		slog.Error("bindings", "err", err)
		return 1
	}
	var node api.Node
	if *fixture != "" {
		node = waves.NewFileNode(*fixture)
	} else {
		node = waves.NewClient(wavesCfg.Nodes.Primary)
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.New(node, j, reg, cfg).Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	slog.Info("api listening", "addr", cfg.ListenAddr)
	if serveErr := srv.ListenAndServe(); serveErr != nil {
		slog.Error("serve", "err", serveErr)
		return 1
	}
	return 0
}
