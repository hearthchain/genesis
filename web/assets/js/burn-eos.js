"use strict";
// Compiled to web/assets/js/burn-eos.js by `make web`. Never edit the compiled file.
// The EOS/Vaulta lane: credit preview by account, copy-the-burn-account, and
// the memo generator. There is no wallet integration: EOS wallets cannot sign
// bare statements, so the binding is a transfer memo the user sends from any
// wallet, and the burn itself is a plain transfer to eosio.null.
(function () {
    const { apiGet, fmtUnits, fmtMicroUsd, errorMessage } = window.HearthAPI;
    const el = (id) => document.getElementById(id);
    const fmtA = (units) => fmtUnits(units, 4); // A and legacy EOS carry 4 decimals
    // --- step 1: preview ---
    const PREVIEW_ERRORS = {
        400: "This does not look like an EOS account name.",
        422: "This account's history needs manual review. Nothing is lost: see the disputes page.",
        429: "The preview lane is busy. Try again in a few seconds.",
        502: "The public EOS nodes are not answering right now. Try again shortly.",
    };
    function layerRow(layer) {
        const tr = document.createElement("tr");
        const cells = [
            [String(layer.since).slice(0, 10), ""], // RFC3339 timestamp: keep the date
            [layer.weekEnd, ""],
            [fmtA(String(layer.amountBaseUnits)), "num"],
            [fmtMicroUsd(String(layer.priceMicroUsd)), "num"],
            [fmtUnits(layer.creditMicro, 6), "num"],
        ];
        for (const [text, cls] of cells) {
            const td = document.createElement("td");
            td.textContent = text;
            if (cls)
                td.className = cls;
            tr.appendChild(td);
        }
        return tr;
    }
    el("preview-form").addEventListener("submit", async (e) => {
        e.preventDefault();
        const account = el("preview-input").value.trim();
        if (!account)
            return;
        const err = el("preview-error");
        err.hidden = true;
        el("preview-result").hidden = true;
        el("preview-go").disabled = true;
        const res = await apiGet("/api/preview/eos/" + encodeURIComponent(account));
        el("preview-go").disabled = false;
        if (!res) {
            err.hidden = false;
            err.textContent = "The API is temporarily unreachable. Your coins and your history are untouched; try again shortly.";
            return;
        }
        if (res.status !== 200) {
            err.hidden = false;
            err.textContent = (PREVIEW_ERRORS[res.status] || "Unexpected response.") + " (" + errorMessage(res) + ")";
            return;
        }
        el("preview-addr").textContent = res.body.address;
        el("preview-credit").textContent = fmtUnits(res.body.minimumCreditMicro, 6);
        const tbody = el("preview-layers");
        tbody.textContent = "";
        for (const layer of res.body.layers || [])
            tbody.appendChild(layerRow(layer));
        el("preview-result").hidden = false;
    });
    // --- step 2: copy the burn account ---
    function bindCopy(buttonId, sourceId, doneId) {
        el(buttonId).addEventListener("click", async () => {
            const text = el(sourceId).textContent.trim();
            try {
                await navigator.clipboard.writeText(text);
            }
            catch {
                const range = document.createRange();
                range.selectNodeContents(el(sourceId));
                const sel = getSelection();
                sel.removeAllRanges();
                sel.addRange(range);
                return; // clipboard blocked: leave the text selected for manual copy
            }
            el(doneId).hidden = false;
            setTimeout(() => { el(doneId).hidden = true; }, 2000);
        });
    }
    bindCopy("copy-address", "burn-address", "copy-done");
    bindCopy("memo-copy", "memo-out", "memo-copy-done");
    // --- step 3: the binding memo generator (offline-pure) ---
    const MESSAGE_PREFIX = "hearth-genesis-binding:v1:";
    const ACCOUNT_SHAPE = /^[a-z1-5](\.?[a-z1-5]){0,11}$/; // Antelope names: a-z, 1-5, interior dots
    const HEARTH_SHAPE = /^[1-9A-HJ-NP-Za-km-z]{30,40}$/; // base58, address-sized; the server verifies the checksum
    el("memo-form").addEventListener("submit", (e) => {
        e.preventDefault();
        const account = el("memo-account").value.trim();
        const hearth = el("memo-hearth").value.trim();
        const err = el("memo-error");
        err.hidden = true;
        el("memo-result").hidden = true;
        if (!ACCOUNT_SHAPE.test(account) || account.length > 12) {
            err.hidden = false;
            err.textContent = "EOS account names are 1-12 characters of a-z, 1-5 and interior dots. Nothing was generated.";
            return;
        }
        if (!HEARTH_SHAPE.test(hearth)) {
            err.hidden = false;
            err.textContent = "This does not look like a Hearth address (base58, ~35 characters, starts with 3H). Nothing was generated.";
            return;
        }
        el("memo-out").textContent = MESSAGE_PREFIX + account + ":" + hearth;
        el("memo-result").hidden = false;
    });
    const params = new URLSearchParams(location.search);
    if (params.get("hearth"))
        el("memo-hearth").value = params.get("hearth");
})();
