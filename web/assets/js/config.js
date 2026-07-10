"use strict";
// Compiled to web/assets/js/config.js by `make web`. Never edit the compiled file.
// The only deployment knob of the static site: where the burn API lives.
// Empty string means same-origin (the site and /api behind one proxy).
// On localhost the API is assumed on :8080 (cmd/api's default listenAddr);
// localStorage.setItem("hearthApiBase", ...) still overrides everything.
(function () {
    const stored = localStorage.getItem("hearthApiBase");
    // A stored override must be an absolute http(s) URL; anything else (a
    // typo'd host without a scheme, stray whitespace) silently breaks every
    // fetch, so fall back to the default instead.
    window.HEARTH_API_BASE = stored && /^https?:\/\//.test(stored.trim())
        ? stored.trim().replace(/\/+$/, "")
        : (["localhost", "127.0.0.1"].includes(location.hostname) ? "http://" + location.hostname + ":8080" : "https://157-230-0-211.sslip.io"); // until api.hearth.tech DNS exists
})();
