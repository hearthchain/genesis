"use strict";
// Compiled to web/assets/js/cabinet.js by `make web`. Never edit the compiled file.
// Cabinet: /a/?<hearth> (bare query is canonical, ?addr=<hearth> accepted).
// No query shows the entry form; with an address it renders credit, bindings
// and the burn table from GET /api/address/{hearth}.
(function () {
  const { apiGet, fmtWaves, fmtUnits, errorMessage, degrade } = window.HearthAPI;
  const el = (id: string) => document.getElementById(id) as HTMLElement;

  const params = new URLSearchParams(location.search);
  const address = (params.get("addr") || decodeURIComponent(location.search.replace(/^\?/, ""))).trim();

  el("cab-open-form").addEventListener("submit", (e) => {
    e.preventDefault();
    const value = (el("cab-input") as HTMLInputElement).value.trim();
    if (value) location.href = "?" + encodeURIComponent(value);
  });

  const STATUS_PILL: Record<string, string> = { confirmed: "pill-ok", mismatch: "pill-err" };

  function burnRow(b: CabinetBurn) {
    const tr = document.createElement("tr");
    const cells: [string, string][] = [
      [b.txId, ""],
      [b.chain, ""],
      [b.source, ""],
      [fmtWaves(String(b.amountBaseUnits)), "num"],
      [String(b.height), "num"],
      [b.creditMicro ? fmtUnits(b.creditMicro, 6) : "·", "num"],
    ];
    for (const [text, cls] of cells) {
      const td = document.createElement("td");
      td.textContent = text;
      if (cls) td.className = cls;
      tr.appendChild(td);
    }
    const td = document.createElement("td");
    const pill = document.createElement("span");
    pill.className = "pill " + (STATUS_PILL[b.status] || "pill-line");
    pill.textContent = b.status;
    td.appendChild(pill);
    tr.appendChild(td);
    return tr;
  }

  function render(body: AddressResponse) {
    el("cab-form").hidden = true;
    el("cab-view").hidden = false;
    el("cab-addr").textContent = body.hearthAddress;
    el("cab-credit").textContent = fmtUnits(body.minimumCreditMicro, 6);
    (el("cab-bind-link") as HTMLAnchorElement).href = "../burn/waves/?hearth=" + encodeURIComponent(body.hearthAddress) + "#bind";

    const bindings = el("cab-bindings");
    if (body.bindings.length === 0) {
      bindings.textContent = "none yet";
    } else {
      for (const source of body.bindings) {
        const div = document.createElement("div");
        div.textContent = source;
        bindings.appendChild(div);
      }
    }

    const burns = (body.burns || []).slice().sort((a, b) => b.height - a.height);
    el("cab-noburns").hidden = burns.length !== 0;
    el("cab-burns-wrap").hidden = burns.length === 0;
    const tbody = el("cab-burns");
    for (const b of burns) tbody.appendChild(burnRow(b));
  }

  async function load() {
    if (!address) return;
    const res = await apiGet<AddressResponse>("/api/address/" + encodeURIComponent(address));
    if (!res) {
      el("cab-form").hidden = true;
      el("cab-view").hidden = false;
      el("cab-addr").textContent = address;
      degrade();
      return;
    }
    if (res.status !== 200) {
      (el("cab-input") as HTMLInputElement).value = address;
      const err = el("cab-error");
      err.hidden = false;
      err.textContent = "This does not look like a valid Hearth address: " + errorMessage(res);
      return;
    }
    render(res.body);
  }

  load();
})();
