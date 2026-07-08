---
purpose: M1-3 architecture and TDD-ready implementation plan for the genesis burn pipeline (Waves mainnet)
---

# Architecture - genesis burn pipeline, milestones 1-3 (Waves mainnet)

## 1. Requirements

Functional: detect burns, that is, plain transfers to the published provably unspendable Waves address on mainnet (no testnet); rebuild per-address holding layers as a min-balance profile from transfer history; compute credits as layer amount times the maximum weekly-average price since the layer date from the canonical journal, with 1 credit = 1 HRTH and oldest-first layer consumption on partial burns; verify signed bindings from source address to Hearth address; emit a per-credit evidence bundle; build a deterministic snapshot with a Merkle root; serve a read/preview API.

Non-functional: every number is reproducible offline from published artifacts, the backend is a cache and never an authority; every burn is cross-checked field by field against two independent public Waves nodes, and a mismatch blocks the credit; serialization is deterministic; code and dependencies stay minimal.

## 2. Technology selection

Selected:

| Choice | Role | Stars | Activity | Why |
|---|---|---|---|---|
| github.com/wavesplatform/gowaves | Waves crypto (Curve25519 signatures, base58 addresses, blake2b/keccak checksums), node REST client, tx types | 254 | v0.11.1, pushed 2026-07-07 | official Go node implementation; we never hand-roll Waves crypto; also the code-style reference |
| Go stdlib net/http | API | - | - | two GET endpoints and one POST need no framework |
| Go stdlib crypto/sha256 | Merkle tree, artifact checksums | - | - | ~30 lines, no dependency |

Rejected:

| Candidate | Why rejected |
|---|---|
| chi, gin | stdlib suffices for three endpoints |
| mattn/go-sqlite3, modernc.org/sqlite | no database at all: data volume is thousands of burns and tens of thousands of transfers; append-only JSONL artifacts plus in-memory maps rebuilt on start are simpler, and the artifacts ARE the published reproducibility bundle |
| hand-rolled Curve25519 | gowaves provides it; signature code is where money dies |

Sources: github.com/wavesplatform/gowaves; docs.waves.tech node REST API reference.

## 3. Patterns

Ports-and-adapters: `internal/chain` defines the ChainAdapter port (`DetectBurns(window)`, `History(address)`, `VerifyBindingSignature(msg, sig, address)`); `internal/chain/waves` is the first adapter, and wave-2 chains plug in without touching the core. Functional core, imperative shell: layers, credit, and Merkle are pure functions with table-driven tests, while IO (node fetch, artifact write) lives at the edges. Pipes: watcher -> JSONL artifacts -> snapshot -> published bundle. No other patterns.

## 4. Architecture

```
two public Waves nodes (independent REST endpoints)
        |
        v
  cmd/watcher            per-chain daemon: burn detection, history fetch, cross-check
        |
        v
  data/ JSONL artifacts  burns.jsonl, transfers/<address>.jsonl, bindings.jsonl
        |                       ^
        v                       | POST /api/bindings (verify signature, append)
  cmd/snapshot            cmd/api
```

`cmd/snapshot` runs layers -> credits -> evidence bundles -> Merkle root, and its `--verify` mode recomputes everything from the artifacts and compares roots. `cmd/api` loads the artifacts into memory and serves GET `/api/preview/waves/<address>` (live fetch plus layer computation), GET `/api/address/<hearth-address>`, GET `/api/stats` (front-page counters: per-chain burn totals, participants, total credit), and POST `/api/bindings`. CORS is answered only for the origins listed in `allowedOrigins` (empty list = no CORS headers). `internal/journal` loads the published weekly price CSV artifact.

## 5. Data layer

No database. Append-only JSONL artifacts with sha256 sums and deterministic ordering, indexed in memory on load; the artifacts double as the published reproducibility bundle. The journal artifact is a CSV exported from the cto-agent price_weekly table (weekly average of daily closes, weeks ending Sunday UTC).

## 6. TDD-ready implementation plan

Three PR-stack blocks, each one PR, strict red-green-refactor per step; each step names its failing test first.

Block 1 - foundation:

1. Burn address: construct and verify the provably unspendable Waves mainnet address (chosen body, valid checksum via gowaves primitives). Test: pins a known-good Waves address checksum and rejects a corrupted one.
2. Binding message: canonical bytes `hearth-genesis-binding:v1:<source>:<hearth>`, Curve25519 signature verification via gowaves. Test: vectors signed with a throwaway key.
3. Journal loader: parse the weekly CSV, `MaxSince(date)` query. Test: pins WAVES $49.71 for the week of 2022-04-03 and $4.00 for 2024-03-17.

Block 2 - watcher:

4. Node client: wrapper over two configurable public node URLs (gowaves client), paginated transfers-by-address. Test: golden tests on recorded mainnet JSON fixtures, no live network.
5. Burn detection: filter transfers to the burn address inside the block window. Test: fixture window with burns and non-burns.
6. Layer reconstruction: running-minimum profile from transfer history. Test: table-driven cases, single deposit, top-up adds a layer, sale trims to the dip, exchange withdrawal starts a fresh layer.
7. Cross-check: field-by-field compare of the two sources, mismatch marks the burn blocked. Test: diverging fixtures.

Block 3 - engine and API:

8. Credit computation: oldest-first layer consumption times journal `MaxSince`; pure function. Test: exhaustive table cases.
9. Evidence bundle: deterministic JSON per credit (burn tx, transfer history refs, layers, prices, formula inputs) plus sha256. Test: byte-identical output on repeated runs.
10. Bindings registry: append-only, signature-verified, latest-wins per source address; unbound burns stay pending. Test: rebind overrides, unbound burn remains pending.
11. Snapshot: aggregate credits by Hearth address, deterministic serialization, sha256 Merkle root over sorted leaves; `--verify` recomputes from artifacts and compares roots. Test: known root on a fixture set, verify round-trip.
12. API: GET preview (live fetch plus layers plus credit), GET address (credits, burns, bindings), POST bindings. Test: httptest coverage of all three endpoints.

Dependencies: steps 1-3 are independent; 4 -> 5 -> 7; 6 is pure once 4's fixtures exist; 8 needs 3 and 6; 9-12 follow 8.

## 7. Open questions

- Burn window block heights (spec).
- Journal artifact publication cadence.
- Second node provider choice (nodes.wavesnodes.com plus which independent one).

## Conventions

Go 1.25.x, dependencies vendored, golangci-lint v2 clean, race-detector tests (`go test -race ./...`), table-driven tests, gowaves code style. KISS/YAGNI: no speculative abstractions beyond the ChainAdapter port.
