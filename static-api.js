/* Static (GitHub Pages) back-end: runs the Go engine as WebAssembly in a Web
   Worker and emulates the desktop app's local HTTP API. */
"use strict";

(function () {
  const LS_KEY = "emif-settings";
  let worker = null, ready = null, seq = 0;
  const pending = new Map();

  function loadSettings() {
    let s = {};
    try { s = JSON.parse(localStorage.getItem(LS_KEY) || "{}") || {}; } catch (e) { s = {}; }
    return Object.assign({ mouserKey: "", digikeyClientId: "", digikeySecret: "", digikeySite: "US", digikeyCurrency: "USD", inStockOnly: false, proxy: "" }, s);
  }
  function storeSettings(s) { try { localStorage.setItem(LS_KEY, JSON.stringify(s)); } catch (e) { throw new Error("Browser storage is not available – keys cannot be saved."); } }
  const mask = (s) => (!s ? "" : s.length <= 6 ? "•".repeat(s.length) : s.slice(0, 3) + "••••••" + s.slice(-3));
  function status(s) {
    return {
      mouser: !!s.mouserKey, digikey: !!(s.digikeyClientId && s.digikeySecret),
      mouserKey: mask(s.mouserKey), digikeyClientId: mask(s.digikeyClientId), digikeySecret: mask(s.digikeySecret),
      digikeySite: s.digikeySite, digikeyCurrency: s.digikeyCurrency, inStockOnly: !!s.inStockOnly, proxy: s.proxy || "",
      configDir: "this browser only (local storage)", static: true,
    };
  }

  async function b64gzToBytes(b64) {
    const bin = atob(b64.replace(/\s+/g, ""));
    const u8 = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) u8[i] = bin.charCodeAt(i);
    const stream = new Blob([u8]).stream().pipeThrough(new DecompressionStream("gzip"));
    return new Response(stream).arrayBuffer();
  }

  function start() {
    if (ready) return ready;
    ready = (async () => {
      const execSrc = document.getElementById("wasm-exec").textContent;
      const boot = `
        self.onmessage = async (e) => {
          const d = e.data;
          if (d.wasm) {
            const go = new Go();
            const { instance } = await WebAssembly.instantiate(d.wasm, go.importObject);
            go.run(instance);
            return;
          }
          let res;
          try { res = self.emifCall(d.fn, d.arg); } catch (err) { res = JSON.stringify({ error: String(err) }); }
          self.postMessage({ id: d.id, res });
        };`;
      const url = URL.createObjectURL(new Blob([execSrc, "\n", boot], { type: "text/javascript" }));
      worker = new Worker(url);
      const wasm = await b64gzToBytes(document.getElementById("wasm-b64").textContent);
      await new Promise((resolve, reject) => {
        worker.onerror = (e) => reject(new Error("Engine failed to start: " + (e.message || "worker error")));
        worker.onmessage = (e) => {
          const d = e.data;
          if (d && d.ready) { resolve(); return; }
          const p = pending.get(d.id);
          if (p) { pending.delete(d.id); p(d.res); }
        };
        worker.postMessage({ wasm }, [wasm]);
      });
      worker.onmessage = (e) => {
        const d = e.data, p = pending.get(d.id);
        if (p) { pending.delete(d.id); p(d.res); }
      };
    })();
    return ready;
  }

  async function wasm(fn, arg) {
    await start();
    const id = ++seq;
    const res = await new Promise((resolve) => { pending.set(id, resolve); worker.postMessage({ id, fn, arg: arg === undefined ? "" : JSON.stringify(arg) }); });
    const j = JSON.parse(res);
    if (j && j.error) throw new Error(j.error);
    return j;
  }

  // ---------------------------------------------------------- distributors
  const viaProxy = (s, url) => (s.proxy ? s.proxy + (s.proxy.includes("?") ? encodeURIComponent(url) : url) : url);
  function netErr(src, e) {
    if (e instanceof TypeError) return new Error(`${src}: the browser blocked the request (the API does not allow calls from web pages / CORS). Use the desktop app for live search, or set an API proxy in Settings.`);
    return e;
  }
  async function mouserSearch(s, kw, n) {
    if (!s.mouserKey) throw new Error("no Mouser API key configured");
    const url = "https://api.mouser.com/api/v1/search/keyword?apiKey=" + encodeURIComponent(s.mouserKey);
    const body = { SearchByKeywordRequest: { keyword: kw, records: n, startingRecord: 0, searchOptions: s.inStockOnly ? "InStock" : "None", searchWithYourSignUpLanguage: "false" } };
    let r;
    try { r = await fetch(viaProxy(s, url), { method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify(body) }); }
    catch (e) { throw netErr("Mouser", e); }
    const raw = await r.text();
    if (!r.ok) throw new Error(`Mouser: HTTP ${r.status}: ${raw.slice(0, 200)}`);
    return (await wasm("parseMouser", { raw })).results || [];
  }
  let dkToken = null;
  async function digikeySearch(s, kw, n) {
    if (!s.digikeyClientId || !s.digikeySecret) throw new Error("no DigiKey client ID / secret configured");
    try {
      if (!dkToken || dkToken.id !== s.digikeyClientId || Date.now() > dkToken.exp) {
        const form = new URLSearchParams({ client_id: s.digikeyClientId, client_secret: s.digikeySecret, grant_type: "client_credentials" });
        const t = await fetch(viaProxy(s, "https://api.digikey.com/v1/oauth2/token"), { method: "POST", body: form });
        const tj = await t.json().catch(() => ({}));
        if (!t.ok || !tj.access_token) throw new Error(`DigiKey auth: HTTP ${t.status}`);
        dkToken = { id: s.digikeyClientId, tok: tj.access_token, exp: Date.now() + Math.max((tj.expires_in || 600) - 60, 60) * 1000 };
      }
      const body = { Keywords: kw, Limit: n, Offset: 0 };
      if (s.inStockOnly) body.FilterOptionsRequest = { SearchOptions: ["InStock"] };
      const r = await fetch(viaProxy(s, "https://api.digikey.com/products/v4/search/keyword"), {
        method: "POST", body: JSON.stringify(body),
        headers: { "Content-Type": "application/json", Accept: "application/json", Authorization: "Bearer " + dkToken.tok,
          "X-DIGIKEY-Client-Id": s.digikeyClientId, "X-DIGIKEY-Locale-Site": s.digikeySite || "US",
          "X-DIGIKEY-Locale-Language": "en", "X-DIGIKEY-Locale-Currency": s.digikeyCurrency || "USD" },
      });
      const raw = await r.text();
      if (r.status === 401) dkToken = null;
      if (!r.ok) throw new Error(`DigiKey: HTTP ${r.status}: ${raw.slice(0, 200)}`);
      return (await wasm("parseDigikey", { raw, currency: s.digikeyCurrency || "USD" })).results || [];
    } catch (e) { throw netErr("DigiKey", e); }
  }

  // ---------------------------------------------------------- API emulation
  window.STATIC_API = async function (path, body) {
    switch (path) {
      case "/api/init": {
        const b = document.getElementById("busy");
        if (b) { b.hidden = false; document.getElementById("busyText").textContent = "Loading simulation engine…"; }
        try {
          const r = await wasm("init");
          return Object.assign(r, { version: "1.0.0 (web)", settings: status(loadSettings()) });
        } finally { if (b) b.hidden = true; }
      }
      case "/api/design": return wasm("design", body);
      case "/api/simulate": return wasm("simulate", body);
      case "/api/applypart": return wasm("applypart", body);
      case "/api/export/spice": return (await wasm("spice", body)).text;
      case "/api/export/bom": return (await wasm("bom", body)).text;
      case "/api/parts": {
        const s = loadSettings();
        let kw = (body.keyword || "").trim();
        if (!kw) kw = (await wasm("keywords", { comp: body.comp })).keyword;
        const errors = {}, counts = {};
        let all = [];
        const jobs = (body.sources || []).map(async (src) => {
          try {
            let r = [];
            if (src === "library") r = (await wasm("library", { comp: body.comp })).results || [];
            else if (src === "mouser") r = await mouserSearch(s, kw, 40);
            else if (src === "digikey") r = await digikeySearch(s, kw, 40);
            counts[src] = r.length;
            all = all.concat(r);
          } catch (e) { errors[src] = e.message; }
        });
        await Promise.all(jobs);
        const ranked = all.length ? (await wasm("rank", { comp: body.comp, results: all })).results : [];
        return { keyword: kw, results: ranked || [], errors, counts };
      }
      case "/api/settings": {
        const s = loadSettings();
        if (body) {
          for (const k of ["mouserKey", "digikeyClientId", "digikeySecret", "digikeySite", "digikeyCurrency", "proxy"]) {
            if (typeof body[k] === "string" && !body[k].includes("•")) s[k] = body[k].trim();
          }
          if (typeof body.inStockOnly === "boolean") s.inStockOnly = body.inStockOnly;
          storeSettings(s);
        }
        return status(s);
      }
      case "/api/testkeys": {
        const s = loadSettings(), out = {};
        out.mouser = s.mouserKey ? await mouserSearch(s, "GRM188R71H104KA93D", 3).then((r) => `OK (${r.length} results)`, (e) => e.message) : "not configured";
        out.digikey = s.digikeyClientId ? await digikeySearch(s, "GRM188R71H104KA93D", 3).then((r) => `OK (${r.length} results)`, (e) => e.message) : "not configured";
        return out;
      }
      case "/api/ping": case "/api/quit": return null;
    }
    throw new Error("unknown API " + path);
  };
  start().catch(() => {}); // begin loading immediately
})();
