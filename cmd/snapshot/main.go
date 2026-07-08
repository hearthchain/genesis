// Command snapshot builds (or verifies) the credit snapshot from artifacts.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/hearthchain/burning-page/internal/config"
	"github.com/hearthchain/burning-page/internal/journal"
	"github.com/hearthchain/burning-page/internal/snapshot"
)

func main() { os.Exit(run()) }

func run() int {
	configPath := flag.String("config", "config.json", "path to the shared config")
	verify := flag.Bool("verify", false, "recompute from artifacts and compare with the stored snapshot.json")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("config", "err", err)
		return 1
	}
	j, err := journal.Load(cfg.JournalCSV)
	if err != nil {
		slog.Error("journal", "err", err)
		return 1
	}
	journals := map[string]*journal.Journal{"waves": j}

	if *verify {
		if vErr := snapshot.Verify(cfg.DataDir, journals, cfg.HearthSchemeByte()); vErr != nil {
			slog.Error("verify", "err", vErr)
			return 1
		}
		_, _ = fmt.Fprintln(os.Stdout, "snapshot verified: rebuilt root matches snapshot.json")
		return 0
	}

	snap, bundles, err := snapshot.Build(cfg.DataDir, journals, cfg.HearthSchemeByte())
	if err != nil {
		slog.Error("build", "err", err)
		return 1
	}
	if wErr := snapshot.Write(cfg.DataDir, snap, bundles); wErr != nil {
		slog.Error("write", "err", wErr)
		return 1
	}
	_, _ = fmt.Fprintf(os.Stdout, "entries=%d totalCreditMicro=%s pending=%d blocked=%d\nmerkleRoot=%s\n",
		len(snap.Entries), snap.TotalCreditMicro, len(snap.PendingSources), len(snap.BlockedSources), snap.Root)
	return 0
}
