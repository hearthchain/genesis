---
purpose: genesis.hearth.tech application, burn-window web app plus backend pipeline (watchers, credit engine, snapshot builder)
---

# burning-page

The genesis burn application for Hearth (HRTH). Users provably burn tokens of ten faded L1 chains during the genesis window and receive HRTH credits, one credit per HRTH token at network launch. This repo holds everything behind genesis.hearth.tech: the static web frontend and the Go backend pipeline that detects burns, computes credits, and builds the genesis snapshot. Mechanics, user path, chain table, and open questions live in the spec: [`hearthchain/strategy/genesis-burn-tz.md`](../strategy/genesis-burn-tz.md).

The backend is a cache, never an authority: every number is recomputable from public chain data plus published artifacts (burn spec, price journal, snapshot with Merkle root and reproduction script).

## Architecture

Single Go monorepo. First chain is Waves; the other nine plug in later through the chain-adapter interface.

- `cmd/api`: read API plus binding submission (chi or stdlib http)
- `cmd/watcher`: per-chain burn detection, address history extraction, layer reconstruction (one process per chain)
- `cmd/snapshot`: deterministic credit computation, evidence bundles, Merkle root, reproduction CLI
- `internal/chain`: `ChainAdapter` interface (`DetectBurns(window)`, `History(address)`, `VerifyBindingSignature(msg, sig, address)`); one package per chain, `internal/chain/waves` first (gowaves client + crypto)
- `internal/layers`: min-balance layer profile from transfer history
- `internal/journal`: canonical weekly price journal (imported artifact, weekly averages of daily closes)
- `internal/bindings`: registry of source-address to Hearth-address bindings
- `web/`: static HTML plus a little TypeScript for wallet integration and client-side Hearth address generation; five pages per the site map: `/`, `/a/<address>`, `/burn`, `/burn/<chain>`, `/disputes`

Storage is SQLite.

## Binding a burn to a Hearth address

Two paths are supported; the decision on their long-term status is tracked in the spec. The MVP ships both for Waves.

1. Payload in the burn transaction: `HRTH1:<address>:<checksum>` in the chain's native data field. Self-contained, no further action.
2. Cabinet binding: a plain burn to the published burn address, then the user binds the source address to a Hearth address by a message signed with the source address key. Signed bindings are published in the evidence bundles.

## MVP plan (Waves)

1. Price-journal artifact, published burn address, binding message format.
2. Waves watcher: burn detection, history, layers, double-source cross-check against two independent public nodes, golden tests on recorded fixtures.
3. Credit engine, evidence bundles, preview API.
4. Web: front page with live counters, address cabinet, `/burn/waves` constructor (Keeper Wallet plus manual path), binding submission.

Then a testnet dry run, then wave-2 chains via the adapter interface.

## Development

Go 1.25.x. Run tests with `go test ./...`. No secrets in the repo. Commits only from the `swell-a2a` identity.
