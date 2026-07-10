---
purpose: Genesis burn application, burn-window web app plus backend pipeline (watchers, credit engine, snapshot builder)
---

# genesis

The genesis burn application for Hearth (HRTH). Users provably burn tokens of ten faded L1 chains during the genesis window and receive HRTH credits, one credit per HRTH token at network launch. This repo holds the whole genesis burn application: the static web frontend and the Go backend pipeline that detects burns, computes credits, and builds the genesis snapshot. Mechanics, user path, chain table, and open questions live in the spec: [`hearthchain/strategy/genesis-burn-tz.md`](../strategy/genesis-burn-tz.md).

The backend is a cache, never an authority: every number is recomputable from public chain data plus published artifacts (burn spec, price journal, snapshot with Merkle root and reproduction script).

## Architecture

Single Go monorepo. Live chains: Waves and EOS/Vaulta; the remaining eight plug in through the chain-adapter port.

- `cmd/api`: read API plus binding submission (stdlib net/http)
- `cmd/watcher`: per-chain burn detection, address history extraction, layer reconstruction (one process per chain, selected with `-chain waves|eos`)
- `cmd/snapshot`: deterministic credit computation, evidence bundles, Merkle root, reproduction CLI
- `internal/chain`: the `Adapter` port (`BurnCandidates`, `CrossCheck`, `History`, `Deltas`, `ValidateAddress`, `Height`) plus the optional `BindingSource` capability for memo-binding chains; `internal/chain/waves` (gowaves client + crypto) and `internal/chain/eos` (Antelope chain API + Hyperion index + Greymass v1 history) implement it, wired by slug in `internal/chain/chains`
- `internal/layers`: min-balance layer profile from transfer history
- `internal/journal`: canonical weekly price journal (imported artifact, weekly averages of daily closes)
- `internal/bindings`: registry of source-address to Hearth-address bindings
- `web/`: static vanilla HTML/CSS/JS, no build step, no external resources; five pages per the site map: `/`, `/a/` (cabinet, address in the query string), `/burn`, `/burn/<chain>`, `/disputes`; in-browser Hearth address generation is a wave-2 item

Storage is append-only JSONL artifacts plus in-memory indexes rebuilt on start; no database (see [`docs/architecture.md`](docs/architecture.md)).

## Binding a burn to a Hearth address

The burn itself is a plain transfer to the published burn address, with no payload required. The user then binds the source address to a Hearth address; one source address maps to exactly one Hearth address, and when several bindings exist the latest one at snapshot freeze wins. Burns from addresses without a binding wait as pending until a binding arrives. The proof differs per chain:

- Waves: the user submits the message `hearth-genesis-binding:v1:<source>:<hearth>` signed with the source address's key (`POST /api/bindings`; Keeper Wallet flow on the site or the `bindsign` CLI).
- EOS/Vaulta: wallets cannot sign bare messages, so the binding rides a transfer memo. Any transfer from the source account to `eosio.null` whose memo is exactly the binding message binds the account: put it in the burn transfer itself, or send a later dust transfer (0.0001 A) once the Hearth address exists. The watcher extracts memos, cross-checks each carrying transaction against the second source, and records format `eos-memo-v1`; the API refuses that format, it is only ever harvested from chain.

## Milestones

1. Foundation: price-journal artifact, published burn address, binding message format.
2. Watcher: mainnet burn detection, history, layers, double-source cross-check against two independent public nodes, golden tests on recorded fixtures.
3. Credit engine, evidence bundles, bindings registry, snapshot with Merkle root, read/preview API.
4. Web: front page with live counters, address cabinet, `/burn/waves` constructor, binding submission.
5. EOS/Vaulta lane: chain-adapter port extraction, EOS watcher (burns of A and legacy EOS to `eosio.null`, both credited 1:1), memo bindings, truncated-floor history, `/burn/eos` page.

Milestones 1-3 are specified in [`docs/architecture.md`](docs/architecture.md). Wave-2 chains follow via the adapter port.

## Deploying the site

`.github/workflows/pages.yml` deploys `web/` to GitHub Pages on every push to `main` that touches `web/**` (plus manual dispatch). The site is a project page under the organization's domain, so it is served at `hearth.tech/<repo-name>/` with no DNS of its own; the one-time repo settings are the Pages build type (GitHub Actions) and the `github-pages` environment's deployment-branch rules. The frontend's API base is the single constant in `web/assets/js/config.js` (empty = same-origin `/api`); until the API is publicly hosted the site degrades to placeholders gracefully.

## Development

Go 1.25.x. Run tests with `go test ./...`. No secrets in the repo. Commits only from the `swell-a2a` identity.

Limitations, all resolving to manual review rather than a wrong credit:

- Waves balance-history support covers transfer-like transactions only (Genesis, Payment, Transfer, MassTransfer); an address whose history contains any other type (lease, DEX, invoke) is blocked. Expanding coverage (lease first) is a deliberate wave-2 work item.
- EOS history is truncated: no free public index covers blocks before 300,000,000 (2023-03-18, the EOS Rio Hyperion floor). Older balances enter as one synthetic opening layer dated at that boundary, priced at the max weekly average since then, which can only understate against true deeper history; the published figure stays a floor, and a deep-history upgrade (own Hyperion or a paid index) can raise credits later, never lower them. Accounts whose numbers do not reconcile (balance moving mid-fetch, index gaps, histories over 50k actions) are blocked.
- The EOS lane leans on single public operators per role (EOS Rio Hyperion for history, Greymass legacy v1 for the cross-check); if either disappears, burns stay pending rather than silently confirming.
