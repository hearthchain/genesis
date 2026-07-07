---
purpose: genesis.hearth.tech application, burn-window web app plus backend pipeline (watchers, credit engine, snapshot builder)
---

# burning-page

The genesis burn application for Hearth (HRTH). Users provably burn tokens of ten faded L1 chains during the genesis window and receive HRTH credits, one credit per HRTH token at network launch. This repo holds everything behind genesis.hearth.tech: the static web frontend and the Go backend pipeline that detects burns, computes credits, and builds the genesis snapshot. Mechanics, user path, chain table, and open questions live in the spec: [`hearthchain/strategy/genesis-burn-tz.md`](../strategy/genesis-burn-tz.md).

The backend is a cache, never an authority: every number is recomputable from public chain data plus published artifacts (burn spec, price journal, snapshot with Merkle root and reproduction script).

## Architecture

Single Go monorepo. First chain is Waves; the other nine plug in later through the chain-adapter interface.

- `cmd/api`: read API plus binding submission (stdlib net/http)
- `cmd/watcher`: per-chain burn detection, address history extraction, layer reconstruction (one process per chain)
- `cmd/snapshot`: deterministic credit computation, evidence bundles, Merkle root, reproduction CLI
- `internal/chain`: `ChainAdapter` interface (`DetectBurns(window)`, `History(address)`, `VerifyBindingSignature(msg, sig, address)`); one package per chain, `internal/chain/waves` first (gowaves client + crypto)
- `internal/layers`: min-balance layer profile from transfer history
- `internal/journal`: canonical weekly price journal (imported artifact, weekly averages of daily closes)
- `internal/bindings`: registry of source-address to Hearth-address bindings
- `web/`: static vanilla HTML/CSS/JS, no build step, no external resources; five pages per the site map: `/`, `/a/` (cabinet, address in the query string), `/burn`, `/burn/<chain>`, `/disputes`; in-browser Hearth address generation is a wave-2 item

Storage is append-only JSONL artifacts plus in-memory indexes rebuilt on start; no database (see [`docs/architecture.md`](docs/architecture.md)).

## Binding a burn to a Hearth address

The burn itself is a plain transfer to the published burn address, with no payload. The user then binds the source address to a Hearth address in the cabinet by submitting a message signed with the source address's key; signed bindings are published in the evidence bundles. One source address maps to exactly one Hearth address: when several signed bindings exist, the latest one at snapshot freeze wins. Burns from addresses without a binding wait in the journal until a binding arrives.

## Milestones (Waves mainnet)

1. Foundation: price-journal artifact, published burn address, binding message format.
2. Watcher: mainnet burn detection, history, layers, double-source cross-check against two independent public nodes, golden tests on recorded fixtures.
3. Credit engine, evidence bundles, bindings registry, snapshot with Merkle root, read/preview API.
4. Web: front page with live counters, address cabinet, `/burn/waves` constructor, binding submission.

Milestones 1-3 are specified in [`docs/architecture.md`](docs/architecture.md). Wave-2 chains follow via the adapter interface.

## Development

Go 1.25.x. Run tests with `go test ./...`. No secrets in the repo. Commits only from the `swell-a2a` identity.

Limitations: MVP balance-history support covers transfer-like transactions only (Genesis, Payment, Transfer, MassTransfer); an address whose history contains any other type (lease, DEX, invoke) is flagged unsupported and blocked to manual review rather than risking a wrong credit. Expanding coverage (lease first) is a deliberate wave-2 work item.
