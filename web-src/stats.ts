"use strict";
// Compiled to web/assets/js/stats.js by `make web`. Never edit the compiled file.
// Front-page counters: poll GET /api/stats once a minute; on failure keep the
// static placeholders and show the degrade notice, never a broken figure.
(function () {
  const { apiGet, fmtWaves, fmtUnits, fmtCredit, degrade } = window.HearthAPI;
  const set = (id: string, text: string) => { const n = document.getElementById(id); if (n) n.textContent = text; };

  async function refresh() {
    const res = await apiGet<StatsResponse>("/api/stats");
    if (!res || res.status !== 200) {
      degrade();
      return;
    }
    const s = res.body;
    // Credit is denominated in max-week dollars and converts 1:1, so the same
    // figure serves both the $-burned and the HRTH-at-genesis tiles.
    set("stat-usd", "$" + fmtCredit(s.totalCredit));
    set("stat-credit", fmtCredit(s.totalCredit));
    const empty = { burnedBaseUnits: "0", pendingBaseUnits: "0" };
    const waves = (s.chains && s.chains.waves) || empty;
    const eos = (s.chains && s.chains.eos) || empty;
    set("stat-burned", fmtWaves(waves.burnedBaseUnits) + " WAVES");
    set("stat-burned-eos", fmtUnits(eos.burnedBaseUnits, 4) + " A");
    const pendingParts = [];
    if (waves.pendingBaseUnits !== "0") pendingParts.push("+" + fmtWaves(waves.pendingBaseUnits) + " WAVES");
    if (eos.pendingBaseUnits !== "0") pendingParts.push("+" + fmtUnits(eos.pendingBaseUnits, 4) + " A");
    set("stat-burned-sub", pendingParts.length ? pendingParts.join(", ") + " awaiting confirmation" : "");
    set("stat-participants", String(s.participants));
    set("stat-bindings-sub", s.bindings + " bound source " + (s.bindings === 1 ? "address" : "addresses"));
    set("stat-root", s.merkleRoot ? "snapshot merkle root: " + s.merkleRoot : "");
    const notice = document.getElementById("api-degraded");
    if (notice) notice.hidden = true;
  }

  refresh();
  setInterval(refresh, 60000);
})();
