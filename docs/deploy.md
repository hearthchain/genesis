---
purpose: Remote API deployment, GitHub variables and one-time server bootstrap for the deploy-api workflow
---

# Deploying the API

`.github/workflows/deploy-api.yml` builds linux/amd64 binaries and ships them to the server over SSH on every backend push to `main`, i.e. on merged PRs (plus manual dispatch). It is inert until `DEPLOY_HOST` is set. Local development is unaffected: the frontend on localhost targets `localhost:8080` by itself, and `make build` / `go run ./cmd/api` work as before.

## GitHub side (Settings → Secrets and variables → Actions)

| Kind | Name | Value |
|---|---|---|
| Variable | `DEPLOY_HOST` | server IP (later: `api.hearth.tech`) |
| Variable | `DEPLOY_USER` | `hearth` |
| Secret | `DEPLOY_SSH_KEY` | private ed25519 key whose public half is in the server's `~hearth/.ssh/authorized_keys` |

## Server bootstrap (one time, as root)

```bash
useradd -m -s /bin/bash hearth
install -d -o hearth -g hearth /opt/hearth /opt/hearth/bin /opt/hearth/bin.new /opt/hearth/data
echo 'hearth ALL=(root) NOPASSWD: /usr/bin/systemctl restart hearth-api hearth-watcher hearth-watcher-eos' > /etc/sudoers.d/hearth-deploy
```

`/etc/systemd/system/hearth-api.service` (per-chain watchers: same unit with `ExecStart=/opt/hearth/bin/watcher -chain waves -config ...` as `hearth-watcher.service` and `-chain eos` as `hearth-watcher-eos.service`):

```ini
[Unit]
Description=Hearth burn API
After=network-online.target

[Service]
User=hearth
WorkingDirectory=/opt/hearth
ExecStart=/opt/hearth/bin/api -config /opt/hearth/config.json
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Then `systemctl daemon-reload && systemctl enable --now hearth-api hearth-watcher hearth-watcher-eos` (first start fails until the operator files below exist; the first workflow run supplies the binaries).

## Operator-managed files (never deployed by the workflow)

- `/opt/hearth/config.json`: production values: per-chain `chains` blocks with real `window`s (waves `confirmations: 100`, eos `0`), `allowedOrigins: ["https://hearth.tech"]`, `dataDir: "data"`. Start from `config.example.json`.
- `/opt/hearth/data/journal/{waves,eos}.csv`: copy once (`make journal` locally, then scp); refresh when the price journals update.
- `/opt/hearth/data/*.jsonl`: the entire state; back it up by cron. Never seed it from a dev machine that ran with `confirmations: 0`.

## TLS

Until a domain points at the server the API is plain `http://<ip>:8080` (fine for testing; browsers block it from an https page). For production put caddy in front: `api.hearth.tech { reverse_proxy localhost:8080 }` and set the frontend base in `web/assets/js/config.js`.
