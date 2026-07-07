#!/bin/sh
# Records mainnet golden fixtures for the waves client tests.
# Run manually from this directory; commits the resulting JSON files.
# Sleeps keep us under the public nodes' rate limits.
set -eu

PRIMARY=https://nodes.wavesnodes.com
SECONDARY=https://nodes.wx.network
ADDR=3PQwxpPWEsHYiFnrncQJNvLmrAXxR454vFy # busy dApp caller: thousands of txs, exercises paging

curl -sf "$PRIMARY/transactions/address/$ADDR/limit/100" -o history_page1.json
sleep 3
AFTER=$(jq -r '.[0][-1].id' history_page1.json)
curl -sf "$PRIMARY/transactions/address/$ADDR/limit/100?after=$AFTER" -o history_page2_after.json
sleep 3
TXID=$(jq -r '.[0][0].id' history_page1.json)
curl -sf "$SECONDARY/transactions/info/$TXID" -o txinfo_secondary.json
sleep 3
curl -sf "$PRIMARY/blocks/height" -o height.json
sleep 3
curl -sf "$PRIMARY/addresses/balance/$ADDR/100" -o balance_confirmations.json
echo "recorded: $(ls -la *.json | wc -l) files"
