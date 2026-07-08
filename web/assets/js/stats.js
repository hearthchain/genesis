"use strict";
// Front-page counters: poll GET /api/stats once a minute; on failure keep the
// static placeholders and show the degrade notice, never a broken figure.
(function () {
  const { apiGet, fmtWaves, fmtCredit, degrade } = window.HearthAPI;
  const set = (id, text) => { const n = document.getElementById(id); if (n) n.textContent = text; };

  async function refresh() {
    const res = await apiGet("/api/stats");
    if (!res || res.status !== 200) {
      degrade();
      return;
    }
    const s = res.body;
    // Credit is denominated in max-week dollars and converts 1:1, so the same
    // figure serves both the $-burned and the HRTH-at-genesis tiles.
    set("stat-usd", "$" + fmtCredit(s.totalCredit));
    set("stat-credit", fmtCredit(s.totalCredit));
    const waves = (s.chains && s.chains.waves) || { burnedWavelets: "0", pendingWavelets: "0" };
    set("stat-burned", fmtWaves(waves.burnedWavelets));
    set("stat-burned-sub", waves.pendingWavelets !== "0" ? "+" + fmtWaves(waves.pendingWavelets) + " awaiting confirmation" : "");
    set("stat-participants", String(s.participants));
    set("stat-bindings-sub", s.bindings + " bound source " + (s.bindings === 1 ? "address" : "addresses"));
    set("stat-root", s.merkleRoot ? "snapshot merkle root: " + s.merkleRoot : "");
    const notice = document.getElementById("api-degraded");
    if (notice) notice.hidden = true;
  }

  refresh();
  setInterval(refresh, 60000);
})();
