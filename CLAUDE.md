---
purpose: Agent baseline for burning-page (genesis.hearth.tech backend + frontend)
---

# burning-page

Genesis burn application for Hearth (HRTH): Go backend that detects burns of faded L1 tokens on their home chains, reconstructs min-balance layers, computes credits at each layer's max weekly-average price, and builds the genesis snapshot; plus the static frontend for genesis.hearth.tech. The backend is a cache, never an authority: every number is recomputable from public chain data plus published JSONL artifacts.

Keep this file short; when you update it, ALWAYS be very laconic.

## Primary references

- [`README.md`](README.md): overview, layout, milestones, limitations.
- [`docs/architecture.md`](docs/architecture.md): backend design, artifacts, invariants.
- [`../strategy/genesis-burn-tz.md`](../strategy/genesis-burn-tz.md): the spec (user story, site map, chain table, framing appendix). SUPERSEDED IN ONE PLACE: the tx-payload binding (`HRTH1:...` in attachment) was dropped; the implemented mechanic is a plain burn to the single burn address + cabinet binding signed by the wallet (see `internal/api/bind.html`).
- [`../hearthchain.github.io/index.html`](../hearthchain.github.io/index.html): style ground truth for `web/` (bitcoin.org-2011 aesthetic, CSS custom properties).

## Verified facts (don't re-derive)

- Waves burn address: `3PHearthBurnXXXXXXXXXXXXXXXXXZgJXd1` (provably unspendable vanity, pinned in `internal/chain/waves/address.go`).
- Binding message `hearth-genesis-binding:v1:<source>:<hearth>`; formats `raw` (bindsign CLI) and `keeper-v1` (Keeper signCustomData envelope `[255,255,255,1] ++ msg`), proven live against a real Keeper extension.
- API: `GET /api/preview/waves/{source}`, `GET /api/address/{hearth}`, `GET /api/stats`, `POST /api/bindings`, `GET /bind`. Credit fields are `minimumCredit`/`minimumCreditMicro`: the max weekly-average price can only grow until snapshot freeze, so the figure is a floor in HRTH terms. CORS only for origins in config `allowedOrigins` (empty = none).
- Burn statuses: pending_confirmations → pending_crosscheck → confirmed | mismatch; burns show immediately, only confirmed ones are credited. Cross-check runs against two independent public nodes.
- History rule: transfer-like tx types only (1/2/4/11); any other type blocks the address to manual review. Lease support is a deliberate wave-2 item.
- Local `config.json` (gitignored) currently runs with `confirmations: 0` for development; before network launch wipe `data/` and re-run the whole window with a real confirmation depth.

## Commands

```bash
make all        # vendor + tidy + fmt-check + lint + test + build
make test       # go test -mod=vendor -race with coverage
make lint       # go vet + golangci-lint v2 strict (.golangci.yml)
make journal    # regenerate data/journal/waves.csv (header included)
```

## Working rules

- TDD per the coding-skills plugin: one failing test, minimal code, refactor, commit. Table-driven tests with testify, external test packages, vendored deps.
- Branch `develop`; never push `main` (hook-blocked). Commit subject: capital start, ≤72 chars, no trailing period, no `Co-Authored-By` or any trailers (hook-enforced). Commits only from the `swell-a2a` identity.
- Framing in ALL user-facing copy (spec appendix): loyalty converts to a share of the new network; credit converts 1:1 to HRTH by the published formula; floating supply set by turnout; NEVER imply a dollar floor or compensation.
- Never ask for seed phrases anywhere; signing happens only in the user's wallet extension. Keeper injects into http(s) pages only, never file://.
- Integer-only arithmetic in the credit path (micro-USD uint64, micro-HRTH big.Int); truncation is the published rule.
- Markdown: YAML frontmatter with `purpose:`, no hard-wrapped prose (one line per paragraph).
