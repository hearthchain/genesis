---
purpose: Agent baseline for burning-page (genesis burn backend + frontend)
---

# burning-page

Genesis burn application for Hearth (HRTH): Go backend that detects burns of faded L1 tokens on their home chains, reconstructs min-balance layers, computes credits at each layer's max weekly-average price, and builds the genesis snapshot; plus the static genesis frontend served at hearth.tech/burning-page/. The backend is a cache, never an authority: every number is recomputable from public chain data plus published JSONL artifacts.

Keep this file short; when you update it, ALWAYS be very laconic.

## Primary references

- [`README.md`](README.md): overview, layout, milestones, limitations.
- [`docs/architecture.md`](docs/architecture.md): backend design, artifacts, invariants.
- [`../strategy/genesis-burn-tz.md`](../strategy/genesis-burn-tz.md): the spec (user story, site map, chain table, framing appendix). SUPERSEDED IN ONE PLACE: the tx-payload binding (`HRTH1:...` in attachment) was dropped; the implemented mechanic is a plain burn to the single burn address + cabinet binding signed by the wallet (see `internal/api/bind.html`).
- [`../hearthchain.github.io/index.html`](../hearthchain.github.io/index.html): style ground truth for `web/` (bitcoin.org-2011 aesthetic, CSS custom properties).

## Verified facts (don't re-derive)

- Waves burn address: `3PHearthBurnXXXXXXXXXXXXXXXXXZgJXd1` (provably unspendable vanity, pinned in `internal/chain/waves/address.go`). EOS burn account: `eosio.null`; both `A` (core.vaulta) and legacy `EOS` (eosio.token) credited 1:1, precision 4, combined-balance histories (swaps net to zero).
- Binding message `hearth-genesis-binding:v1:<source>:<hearth>`; formats `raw` (bindsign CLI) and `keeper-v1` (Keeper signCustomData envelope `[255,255,255,1] ++ msg`), proven live against a real Keeper extension. EOS binds via that exact message as a transfer memo to eosio.null (on the burn itself or a later 0.0001 A dust transfer; latest wins); watcher-harvested as `eos-memo-v1` through `Registry.AddVerified`; `POST /api/bindings` MUST keep rejecting that format.
- EOS sources: chain API primary (EOS Nation), Greymass v1 history secondary (cross-check), Hyperion `historyAPI` (EOS Rio; only free index, floor block 300,000,000 = 2023-03-18). Pre-floor balances = synthetic opening layer at the floor (truncated-floor rule; credits only ever grow). `confirmations: 0` because Height() is LIB (Savanna, ~1s finality).
- API: `GET /api/preview/{chain}/{source}`, `GET /api/address/{hearth}`, `GET /api/stats` (`windows` per chain), `POST /api/bindings`, `GET /bind`. Credit fields are `minimumCredit`/`minimumCreditMicro`: the max weekly-average price can only grow until snapshot freeze, so the figure is a floor in HRTH terms. CORS only for origins in config `allowedOrigins` (empty = none).
- Burn statuses: pending_confirmations → pending_crosscheck → confirmed | mismatch; burns show immediately, only confirmed ones are credited. Cross-check runs against an independent second source per chain.
- History rules: Waves transfer-like tx types only (1/2/4/11); EOS native-token transfers only, 50k-action cap, double balance read. Anything unprovable blocks the address to manual review.
- Config is per-chain blocks under `chains`; the watcher runs one process per chain (`-chain waves|eos`). Local `config.json` (gitignored) currently runs with `confirmations: 0` for development; before network launch wipe `data/` and re-run the whole window with a real confirmation depth.

## Commands

```bash
make all        # vendor + tidy + fmt-check + lint + test + build
make test       # go test -mod=vendor -race with coverage
make lint       # go vet + golangci-lint v2 strict (.golangci.yml)
make journal    # regenerate data/journal/{waves,eos}.csv (headers included)
make web        # compile web-src/*.ts -> web/assets/js (tsc, pinned)
make web-check  # rebuild + fail on drift (mirrors the CI web job)
```

## Working rules

- TDD per the coding-skills plugin: one failing test, minimal code, refactor, commit. Table-driven tests with testify, external test packages, vendored deps.
- Branch `develop`; never push `main` (hook-blocked). Commit subject: capital start, ≤72 chars, no trailing period, no `Co-Authored-By` or any trailers (hook-enforced). Commits only from the `swell-a2a` identity.
- Framing in ALL user-facing copy (spec appendix): loyalty converts to a share of the new network; credit converts 1:1 to HRTH by the published formula; floating supply set by turnout; NEVER imply a dollar floor or compensation.
- Frontend JS is compiled from `web-src/*.ts` (tsc, module "none", plain global scripts): edit the .ts, never `web/assets/js/*.js`; commit the regenerated JS with the .ts change.
- Never ask for seed phrases anywhere; signing happens only in the user's wallet extension. Keeper injects into http(s) pages only, never file://.
- Integer-only arithmetic in the credit path (micro-USD uint64, micro-HRTH big.Int); truncation is the published rule.
- Markdown: YAML frontmatter with `purpose:`, no hard-wrapped prose (one line per paragraph).
