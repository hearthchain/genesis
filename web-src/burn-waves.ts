"use strict";
// Compiled to web/assets/js/burn-waves.js by `make web`. Never edit the compiled file.
// The Waves lane: credit preview by source address, copy-the-burn-address,
// one-click burn via Keeper, and the Keeper binding flow (same statement and
// envelope as the embedded /bind page, but against the configured API base).
(function () {
  const { apiGet, apiPost, fmtWaves, fmtUnits, fmtMicroUsd, errorMessage } = window.HearthAPI;
  const el = (id: string) => document.getElementById(id) as HTMLElement;

  // --- step 1: preview ---

  const PREVIEW_ERRORS: Record<number, string> = {
    400: "This does not look like a Waves mainnet address.",
    422: "This address's history needs manual review (it contains more than plain transfers). Nothing is lost: see the disputes page.",
    429: "The preview lane is busy. Try again in a few seconds.",
    502: "The public Waves nodes are not answering right now. Try again shortly.",
  };

  function layerRow(layer: PreviewLayer) {
    const tr = document.createElement("tr");
    const cells: [string, string][] = [
      [String(layer.since).slice(0, 10), ""], // RFC3339 timestamp: keep the date
      [layer.weekEnd, ""],
      [fmtWaves(String(layer.amountBaseUnits)), "num"],
      [fmtMicroUsd(String(layer.priceMicroUsd)), "num"],
      [fmtUnits(layer.creditMicro, 6), "num"],
    ];
    for (const [text, cls] of cells) {
      const td = document.createElement("td");
      td.textContent = text;
      if (cls) td.className = cls;
      tr.appendChild(td);
    }
    return tr;
  }

  el("preview-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const address = (el("preview-input") as HTMLInputElement).value.trim();
    if (!address) return;
    const err = el("preview-error");
    err.hidden = true;
    el("preview-result").hidden = true;
    (el("preview-go") as HTMLButtonElement).disabled = true;
    const res = await apiGet<PreviewResponse>("/api/preview/waves/" + encodeURIComponent(address));
    (el("preview-go") as HTMLButtonElement).disabled = false;
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
    for (const layer of res.body.layers || []) tbody.appendChild(layerRow(layer));
    el("preview-result").hidden = false;
  });

  // --- step 2: copy the burn address ---

  el("copy-address").addEventListener("click", async () => {
    const address = el("burn-address").textContent!.trim();
    try {
      await navigator.clipboard.writeText(address);
    } catch {
      const range = document.createRange();
      range.selectNodeContents(el("burn-address"));
      const sel = getSelection()!;
      sel.removeAllRanges();
      sel.addRange(range);
      return; // clipboard blocked: leave the address selected for manual copy
    }
    el("copy-done").hidden = false;
    setTimeout(() => { el("copy-done").hidden = true; }, 2000);
  });

  // --- step 3: bind via Keeper ---

  const MESSAGE_PREFIX = "hearth-genesis-binding:v1:";
  const HEARTH_SHAPE = /^[1-9A-HJ-NP-Za-km-z]{30,40}$/; // base58, address-sized; the server verifies the checksum
  let source: string | null = null;

  function showBind(kind: string, text: string) {
    const r = el("bind-result");
    r.hidden = false;
    r.className = "result " + kind;
    r.textContent = text;
  }

  async function waitKeeper(tries: number): Promise<KeeperWalletApi | null> {
    for (let i = 0; i < tries; i++) {
      if (window.KeeperWallet) return window.KeeperWallet;
      await new Promise((res) => setTimeout(res, 150));
    }
    return null;
  }

  async function initBind() {
    const params = new URLSearchParams(location.search);
    if (params.get("hearth")) (el("bind-hearth") as HTMLInputElement).value = params.get("hearth")!;

    const keeper = await waitKeeper(20);
    if (!keeper) {
      el("bind-source").textContent = "Keeper Wallet not found: install the extension and reload, or use the bindsign CLI below";
      return;
    }
    try {
      const state = await keeper.publicState();
      source = state.account && state.account.address;
      if (!source) throw new Error("no active account in Keeper");
      el("bind-source").textContent = source;
      (el("bind-sign") as HTMLButtonElement).disabled = false;
      (el("burn-keeper") as HTMLButtonElement).disabled = false;
    } catch (e) {
      el("bind-source").textContent = "Keeper refused access: " + (e && e.message ? e.message : e);
    }
  }

  // --- burn via Keeper: a plain type-4 transfer signed and published by the
  // extension, so the address is never copied by hand ---

  const AMOUNT_SHAPE = /^\d+(\.\d{1,8})?$/; // WAVES has 8 decimals

  function showBurn(kind: string, text: string) {
    const r = el("burn-result");
    r.hidden = false;
    r.className = "result " + kind;
    r.textContent = text;
  }

  el("burn-keeper").addEventListener("click", async () => {
    const amount = (el("burn-amount") as HTMLInputElement).value.trim();
    if (!AMOUNT_SHAPE.test(amount) || !/[1-9]/.test(amount)) {
      showBurn("err", "Enter the amount in WAVES, e.g. 10 or 0.5. Nothing was sent.");
      return;
    }
    (el("burn-keeper") as HTMLButtonElement).disabled = true;
    try {
      const published = await window.KeeperWallet!.signAndPublishTransaction({
        type: 4,
        data: {
          recipient: el("burn-address").textContent!.trim(),
          amount: { assetId: "WAVES", tokens: amount },
          fee: { assetId: "WAVES", tokens: "0.001" },
        },
      });
      let txId = "";
      try { txId = JSON.parse(published).id || ""; } catch { /* Keeper answered non-JSON: the link below is just skipped */ }
      showBurn("ok", "Burned " + amount + " WAVES." + (txId ? "\nTransaction: " + txId : "") + "\nIt shows in your cabinet within minutes and is credited once confirmed on two independent nodes.");
    } catch (e) {
      showBurn("err", "Keeper did not publish the transfer: " + (e && e.message ? e.message : e));
    } finally {
      (el("burn-keeper") as HTMLButtonElement).disabled = false;
    }
  });

  el("bind-sign").addEventListener("click", async () => {
    const hearth = (el("bind-hearth") as HTMLInputElement).value.trim();
    if (!hearth) { showBind("err", "Enter the Hearth address first."); return; }
    if (!HEARTH_SHAPE.test(hearth)) {
      showBind("err", "This does not look like a Hearth address (base58, ~35 characters, starts with 3H). Nothing was signed.");
      return;
    }
    const message = MESSAGE_PREFIX + source + ":" + hearth;
    const b64 = btoa(String.fromCharCode(...new TextEncoder().encode(message)));
    try {
      const signed = await window.KeeperWallet!.signCustomData({ version: 1, binary: "base64:" + b64 });
      const res = await apiPost<ErrorEnvelope>("/api/bindings", {
        source: source,
        hearth: hearth,
        publicKey: signed.publicKey,
        signature: signed.signature,
        format: "keeper-v1",
      });
      if (!res) {
        showBind("err", "Signed, but the API is unreachable. Nothing is lost: the signed statement can be resubmitted any time before the snapshot freezes.");
        return;
      }
      if (res.status === 201) {
        showBind("ok", "Binding accepted.\nSigned statement: " + message);
        const holder = el("bind-cabinet");
        holder.textContent = "Every burn from this source now credits your cabinet: ";
        const link = document.createElement("a");
        link.href = "../../a/?" + encodeURIComponent(hearth);
        link.textContent = "open it";
        holder.appendChild(link);
      } else {
        showBind("err", "Rejected (" + res.status + "): " + errorMessage(res));
      }
    } catch (e) {
      showBind("err", "Keeper signing failed: " + (e && e.message ? e.message : e));
    }
  });

  initBind();
})();
