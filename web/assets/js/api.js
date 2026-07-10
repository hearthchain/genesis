"use strict";
// Compiled to web/assets/js/api.js by `make web`. Never edit the compiled file.
// Small fetch + formatting helpers shared by every live page. All amounts
// arrive as decimal strings (base units, micro-HRTH); they never pass through
// Number(), only string arithmetic, so nothing is rounded in the browser.
(function () {
    const base = window.HEARTH_API_BASE || "";
    // apiGet returns {status, body} with the parsed JSON (the API answers JSON
    // for errors too), or null when the API is unreachable: callers degrade.
    async function apiGet(path) {
        try {
            const resp = await fetch(base + path, { headers: { Accept: "application/json" } });
            return { status: resp.status, body: await resp.json() };
        }
        catch {
            return null;
        }
    }
    async function apiPost(path, payload) {
        try {
            const resp = await fetch(base + path, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(payload),
            });
            return { status: resp.status, body: await resp.json() };
        }
        catch {
            return null;
        }
    }
    // fmtUnits("100000000000", 8) -> "1,000": shifts the decimal point left by
    // `decimals` digits, trims trailing zeros, groups thousands.
    function fmtUnits(str, decimals) {
        const digits = String(str).replace(/^0+(?=\d)/, "").padStart(decimals + 1, "0");
        const whole = digits.slice(0, digits.length - decimals).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
        const frac = digits.slice(digits.length - decimals).replace(/0+$/, "");
        return frac ? whole + "." + frac : whole;
    }
    const fmtWaves = (wavelets) => fmtUnits(wavelets, 8);
    // The API pre-renders credit as "49713.174000"; regroup and trim for display.
    function fmtCredit(decimalStr) {
        const [whole, frac = ""] = String(decimalStr).split(".");
        return fmtUnits(whole + frac.padEnd(6, "0"), 6);
    }
    const fmtMicroUsd = (micro) => fmtUnits(String(micro), 6);
    // errorMessage digs the API error envelope out of an apiGet/apiPost result.
    function errorMessage(res) {
        return (res && res.body && res.body.error && res.body.error.message) || "unexpected response";
    }
    // degrade reveals the shared "figures unavailable" notice of a page.
    function degrade() {
        const el = document.getElementById("api-degraded");
        if (el)
            el.hidden = false;
    }
    window.HearthAPI = { apiGet, apiPost, fmtUnits, fmtWaves, fmtCredit, fmtMicroUsd, errorMessage, degrade };
})();
