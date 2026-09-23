/* EMI Filter Designer - UI logic */
"use strict";

const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => Array.from(r.querySelectorAll(s));
const esc = (s) => String(s == null ? "" : s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

// ---------------------------------------------------------------- numbers
const PREF = [[1e9, "G"], [1e6, "M"], [1e3, "k"], [1, ""], [1e-3, "m"], [1e-6, "µ"], [1e-9, "n"], [1e-12, "p"], [1e-15, "f"]];
function fmtEng(v, unit = "", digits = 3) {
  if (v == null || !isFinite(v)) return "–";
  if (v === 0) return "0 " + unit;
  const a = Math.abs(v);
  for (const [m, p] of PREF) if (a >= m * 0.9995) return Number((v / m).toPrecision(digits)) + " " + p + unit;
  return v.toPrecision(digits) + " " + unit;
}
function fmtEngPlain(v) { // for input boxes: "4.7u", "400k"
  if (v == null || !isFinite(v)) return "";
  if (v === 0) return "0";
  const a = Math.abs(v);
  for (const [m, p] of PREF) if (a >= m * 0.9995) return Number((v / m).toPrecision(4)) + (p === "µ" ? "u" : p);
  return String(v);
}
function parseEng(s) {
  if (typeof s === "number") return s;
  s = String(s).trim().replace(",", ".").replace(/\s+/g, "");
  const m = s.match(/^([-+]?\d*\.?\d+(?:e[-+]?\d+)?)(meg|Meg|MEG|[pnuµμmkKMG])?([a-zA-ZΩ%]*)$/);
  if (!m) return NaN;
  let v = parseFloat(m[1]);
  const p = m[2] || "";
  const mult = { p: 1e-12, n: 1e-9, u: 1e-6, "µ": 1e-6, "μ": 1e-6, m: 1e-3, k: 1e3, K: 1e3, M: 1e6, meg: 1e6, Meg: 1e6, MEG: 1e6, G: 1e9 }[p];
  if (mult) v *= mult;
  return v;
}
const fmtDb = (v) => (v == null || !isFinite(v) ? "–" : (v >= 0 ? "+" : "−") + Math.abs(v).toFixed(1) + " dB");

// ---------------------------------------------------------------- state
const S = {
  init: null, mode: "dc",
  params: { dc: null, ac: null }, circuit: { dc: null, ac: null }, sim: { dc: null, ac: null }, log: { dc: [], ac: [] },
  expanded: {}, spiceStale: true, reqSeq: 0,
};
const P = () => S.params[S.mode];
const C = () => S.circuit[S.mode];
const SIM = () => S.sim[S.mode];

// ---------------------------------------------------------------- API
async function api(path, body) {
  if (window.STATIC_API) return window.STATIC_API(path, body);
  const r = await fetch(path, body === undefined ? {} : { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  const ct = r.headers.get("content-type") || "";
  if (!r.ok) {
    let msg = r.statusText;
    try { msg = (await r.json()).error || msg; } catch (e) { /* ignore */ }
    throw new Error(msg);
  }
  if (ct.includes("json")) return r.json();
  return r.text();
}
let busyN = 0;
function busy(on, text) {
  busyN += on ? 1 : -1;
  $("#busy").hidden = busyN <= 0;
  if (text) $("#busyText").textContent = text;
}
function toast(msg, ms = 2600) {
  const t = $("#toast"); t.textContent = msg; t.hidden = false;
  clearTimeout(toast._t); toast._t = setTimeout(() => (t.hidden = true), ms);
}

// ---------------------------------------------------------------- form
const TOPO = {
  dc: [["buck", "Buck"], ["boost", "Boost"], ["buckboost", "Buck-boost"], ["flyback", "Flyback / forward"]],
  ac: [["flyback", "Flyback (bulk cap after bridge)"], ["pfc", "Boost PFC (CCM)"]],
};
const isTopo = (...t) => (p) => t.includes(p.topology);
const FIELDS = [
  { sec: "Compliance target" },
  { key: "standard", type: "select", label: "Standard", wide: true, options: () => S.init.standards.filter((s) => S.mode === "dc" || s.lisn === "50uH").map((s) => [s.id, s.name]) },
  { key: "detector", type: "select", label: "Detector limit", options: () => [["av", "Average"], ["qp", "Quasi-peak"], ["pk", "Peak"]], help: "Switching harmonics are narrow-band, so PK, QP and AV readings are almost equal – the AV limit is the one that governs." },
  { key: "marginDb", label: "EMI margin", unit: "dB", help: "Required distance below the limit (design target). 6 dB is a common pre-compliance margin." },
  { key: "stabMarginDb", label: "Stability margin", unit: "dB", help: "Middlebrook: peak |Zout| of the filter must be this far below the converter's negative input impedance |Zin| = Vmin²/Pin." },

  { sec: "Converter" },
  { key: "topology", type: "select", label: "Topology", options: () => TOPO[S.mode] },
  { key: "pout", label: "Output power", unit: "W" },
  { key: "eff", label: "Efficiency", unit: "%", scale: 0.01 },
  { key: "fsw", label: "Switching freq.", unit: "Hz", eng: true },
  { key: "vout", label: "Output voltage", unit: "V", show: (p) => S.mode === "dc" && ["buck", "boost", "buckboost"].includes(p.topology), help: "Sets the duty cycle (buck/boost) of the noise model." },
  { key: "vout", label: "PFC bus voltage", unit: "V", show: (p) => S.mode === "ac" && p.topology === "pfc", help: "Boost PFC output (typ. 390–400 V)." },
  { key: "vrefl", label: "Reflected voltage", unit: "V", show: isTopo("flyback"), help: "Flyback: n·Vout reflected to the primary. Sets duty cycle and drain voltage swing." },
  { key: "rippleRatio", label: "Inductor ripple", unit: "%", scale: 0.01, show: isTopo("boost", "pfc"), help: "Peak-to-peak inductor ripple relative to the average input current." },

  { sec: "Input" },
  { key: "vinMin", label: "Vin min", unit: "V", dc: true },
  { key: "vinNom", label: "Vin nominal", unit: "V", dc: true },
  { key: "vinMax", label: "Vin max", unit: "V", dc: true },
  { key: "vinAbsMax", label: "Transient max", unit: "V", dc: true, help: "Highest voltage the filter sees (load dump, surge). Used for capacitor voltage ratings (×1.25)." },
  { key: "vacMin", label: "Vac min", unit: "V", ac: true },
  { key: "vacMax", label: "Vac max", unit: "V", ac: true },
  { key: "lineFreq", label: "Line frequency", unit: "Hz", ac: true },
  { key: "pf", label: "Power factor", unit: "", ac: true, help: "Sets the rms line current (CM choke rating). ~0.5–0.6 without PFC, ~0.98 with PFC." },

  { sec: "Noise model" },
  { key: "riseTime", label: "Switch rise time", unit: "s", eng: true, help: "Switch-node transition time; sets the high-frequency roll-off of the noise spectrum." },
  { key: "cp", label: "C coupling (Cp)", unit: "F", eng: true, help: "Effective capacitance from the switch node to chassis / PE (heatsink, transformer inter-winding, PCB to reference plane). Typical: 1–5 pF for a small DC/DC above a ground plane, 5–30 pF for off-line supplies." },
  { key: "cinC", label: "Converter C_in", unit: "F", eng: true, help: "Input capacitance on the converter itself (not part of the filter BOM). AC: bulk capacitor after the bridge (flyback) or film cap (PFC)." },
  { key: "cinEsr", label: "C_in ESR", unit: "Ω", eng: true },
  { key: "cinEsl", label: "C_in ESL", unit: "H", eng: true },

  { sec: "Filter options" },
  { key: "stages", type: "select", label: "DM/CM stages", options: () => [["auto", "Automatic"], ["1", "1 stage"], ["2", "2 stages"]] },
  { key: "cmFilter", type: "select", label: "CM choke", options: () => [["auto", "When needed"], ["on", "Always"], ["off", "Never"]] },
  { key: "returnGrounded", type: "check", label: "Return line grounded at the EUT (single LISN)", dc: true },
  { key: "allowY", type: "check", label: "Chassis available for line-to-chassis caps", dc: true },
  { key: "maxCyDc", label: "Line–chassis C", unit: "F", eng: true, dc: true, show: (p) => p.allowY },
  { key: "leakageLimit", label: "Leakage budget", unit: "mA", scale: 1e-3, ac: true, help: "Touch / protective-conductor current allowed for the Y capacitors at Vac max (e.g. 0.25 mA class II, 0.5–3.5 mA class I)." },
  { key: "maxCy", label: "Max Y cap / line", unit: "F", eng: true, ac: true },
  { key: "maxCx", label: "Max X cap", unit: "F", eng: true, ac: true },
  { key: "leakRatio", label: "CM choke leakage", unit: "%", scale: 0.01, ac: true, help: "DM leakage inductance as a fraction of the CM inductance (typ. 0.5–2 %). It provides the DM inductance of the filter." },
];

function fieldVisible(f) {
  if (f.dc && S.mode !== "dc") return false;
  if (f.ac && S.mode !== "ac") return false;
  if (f.show && !f.show(P())) return false;
  return true;
}
function displayVal(f, v) {
  if (f.eng) return fmtEngPlain(v);
  const x = v / (f.scale || 1);
  return String(Number(x.toPrecision(6)));
}

function renderForm() {
  const side = $("#side");
  const openState = {};
  $$("details.sec", side).forEach((d) => (openState[d.dataset.sec] = d.open));
  side.innerHTML = "";
  let body = null;
  FIELDS.forEach((f, idx) => {
    if (f.sec) {
      const d = document.createElement("details");
      d.className = "sec"; d.dataset.sec = f.sec;
      d.open = openState[f.sec] !== undefined ? openState[f.sec] : true;
      d.innerHTML = `<summary>${esc(f.sec)}</summary><div class="body"></div>`;
      side.appendChild(d);
      body = $(".body", d);
      return;
    }
    if (!fieldVisible(f)) return;
    const p = P();
    const row = document.createElement("div");
    const help = f.help ? `<span class="q" title="${esc(f.help)}">?</span>` : "";
    if (f.type === "check") {
      row.className = "field wide";
      row.innerHTML = `<label class="chk"><input type="checkbox" ${p[f.key] ? "checked" : ""}> ${esc(f.label)} ${help}</label>`;
      $("input", row).addEventListener("change", (e) => { p[f.key] = e.target.checked; paramsChanged(true); });
    } else if (f.type === "select") {
      row.className = "field" + (f.wide ? " wide" : "");
      const opts = f.options().map(([v, t]) => `<option value="${esc(v)}" ${String(p[f.key]) === v ? "selected" : ""}>${esc(t)}</option>`).join("");
      row.innerHTML = `${f.wide ? "" : `<label>${esc(f.label)} ${help}</label>`}<select aria-label="${esc(f.label)}">${opts}</select>`;
      $("select", row).addEventListener("change", (e) => { p[f.key] = e.target.value; paramsChanged(true); });
    } else {
      row.className = "field";
      row.innerHTML = `<label for="f${idx}">${esc(f.label)} ${help}</label><div class="inp"><input id="f${idx}" type="text" spellcheck="false" value="${esc(displayVal(f, p[f.key]))}"><span class="unit">${esc(f.unit || "")}</span></div>`;
      const inp = $("input", row);
      const commit = () => {
        let v = parseEng(inp.value);
        if (!isFinite(v) || v < 0) { inp.classList.add("bad"); return; }
        inp.classList.remove("bad");
        if (!f.eng) v *= f.scale || 1;
        if (p[f.key] !== v) { p[f.key] = v; paramsChanged(false); }
        inp.value = displayVal(f, v);
      };
      inp.addEventListener("change", commit);
      inp.addEventListener("keydown", (e) => { if (e.key === "Enter") { commit(); inp.blur(); } });
    }
    body.appendChild(row);
  });
}

let resimTimer = null;
function paramsChanged(rerenderForm) {
  if (rerenderForm) renderForm();
  if (!C()) return;
  clearTimeout(resimTimer);
  resimTimer = setTimeout(() => simulate(), 450);
}

// ---------------------------------------------------------------- actions
async function design() {
  const seq = ++S.reqSeq;
  busy(true, "Designing filter…");
  try {
    const r = await api("/api/design", { params: P() });
    if (seq !== S.reqSeq) return;
    S.circuit[S.mode] = r.circuit; S.sim[S.mode] = r.sim; S.log[S.mode] = r.log || [];
    S.expanded = {};
    S.spiceStale = true;
    renderAll();
  } catch (e) { toast("Design failed: " + e.message, 5000); }
  finally { busy(false); }
}

async function simulate() {
  if (!C()) return design();
  const seq = ++S.reqSeq;
  busy(true, "Simulating…");
  try {
    const r = await api("/api/simulate", { params: P(), circuit: C() });
    if (seq !== S.reqSeq) return;
    S.sim[S.mode] = r.sim;
    S.spiceStale = true;
    renderAll();
  } catch (e) { toast("Simulation failed: " + e.message, 5000); }
  finally { busy(false); }
}

// ---------------------------------------------------------------- rendering
function renderAll() {
  renderCards();
  renderBanner();
  renderSchematic();
  renderCharts();
  renderBOM();
  renderLog();
  if ($(".tabs .on").dataset.tab === "spice") loadSpice();
}

function card(k, v, s, cls) { return `<div class="card ${cls || ""}"><div class="k">${k}</div><div class="v">${v}</div><div class="s" title="${esc(s)}">${s}</div></div>`; }

function renderCards() {
  const r = SIM(), c = C();
  if (!r || !c) { $("#cards").innerHTML = ""; return; }
  const req = r.required;
  const pass = r.margin >= req;
  const bom = c.comps.filter((x) => x.inBom);
  const nParts = bom.reduce((a, x) => a + (x.qty || 1), 0);
  let cost = 0, priced = 0, cur = "";
  for (const x of bom) if (x.part && x.part.unitPrice > 0) { cost += x.part.unitPrice * (x.qty || 1); priced++; cur = x.part.currency || cur; }
  const costTxt = priced ? `${cost.toFixed(2)} ${cur}` : `${bom.length} lines`;
  const costSub = priced ? `${priced}/${bom.length} lines priced (qty 1)` : "Pick parts via Mouser/DigiKey for pricing";
  $("#cards").innerHTML =
    card("Emission margin", fmtDb(r.margin) + `<span class="pill ${pass ? "ok" : "bad"}">${pass ? "PASS" : "FAIL"}</span>`,
      `worst at ${fmtEng(r.marginF, "Hz")} · target ≥ ${req} dB`, pass ? "pass" : "fail") +
    card("DM / CM margin", `${fmtDb(r.marginDm).replace(" dB", "")}<span style="color:#94a3b8"> / </span>${fmtDb(r.marginCm)}`,
      `unfiltered ${fmtDb(r.unfMargin)} · ${r.standard.name}`, "") +
    card("Stability", fmtDb(r.stab.marginDb) + `<span class="pill ${r.stab.pass ? "ok" : "bad"}">${r.stab.pass ? "OK" : "LOW"}</span>`,
      `|Zout| peak ${r.stab.peakDb.toFixed(1)} dBΩ @ ${fmtEng(r.stab.peakF, "Hz")} · |Zin| ${fmtEng(r.stab.zin, "Ω")}`, r.stab.pass ? "pass" : "fail") +
    card("Filter drop / loss", `${fmtEng(r.dropV, "V")}`, `${fmtEng(r.lossW, "W")} at ${fmtEng(r.irms, "A")} ${S.mode === "ac" ? "rms" : "DC"}`, "") +
    card("Parts", `${nParts}`, `${costTxt} · ${costSub}`, "");
}

function renderBanner() {
  const r = SIM();
  const b = $("#banner");
  if (!r || !r.warnings || !r.warnings.length) { b.hidden = true; return; }
  b.hidden = false;
  b.innerHTML = `<b>Check:</b><ul>${r.warnings.map((w) => `<li>${esc(w)}</li>`).join("")}</ul>`;
}

function renderSchematic() {
  const c = C(), r = SIM();
  if (!c) return;
  c._lisn = r && r.standard.lisn === "5uH" ? "5 µH / 50 Ω" : "50 µH / 50 Ω";
  drawSchematic($("#schematic"), c, S.mode, r && r.singleLisn, (ref) => gotoPart(ref));
  const notes = [`<div class="desc">${esc(c.description || "")}</div>`];
  for (const n of c.notes || []) notes.push(`<div>• ${esc(n)}</div>`);
  if (S.mode === "dc") notes.push(`<div class="muted">Place CHF caps right at the connector, keep the filter away from the switching loop, and use a solid ground plane under the filter. Click a component to edit it or pick a real part.</div>`);
  else notes.push(`<div class="muted">X capacitors must be X2 (or X1) rated, line-to-PE capacitors Y1/Y2 rated. Observe creepage/clearance for mains. Click a component to edit it or pick a real part.</div>`);
  $("#schemNotes").innerHTML = notes.join("");
}

function gotoPart(ref) {
  switchTab("bom");
  S.expanded[ref] = true;
  renderBOM();
  const row = $(`#bomTable tr[data-ref="${ref}"]`);
  if (row) { row.scrollIntoView({ block: "center" }); row.style.transition = "background 1s"; row.querySelectorAll("td").forEach((td) => { td.style.background = "#dbeafe"; setTimeout(() => (td.style.background = ""), 900); }); }
}

// ---------------------------------------------------------------- charts
const COL = { total: "#2563eb", dm: "#7c3aed", cm: "#0d9488", unf: "#94a3b8", av: "#dc2626", qp: "#f59e0b", pk: "#a16207", tgt: "#fca5a5", zin: "#dc2626", zund: "#94a3b8" };
const charts = {};
const hidden = { emi: {}, att: {}, stab: {} };

function limitSeries(std) {
  const mk = (d) => std.segs.map((g) => ({ x1: g.f1, x2: g.f2, y1: g[d + "1"], y2: g[d + "2"] }));
  return mk;
}
function limitAt(std, f, det) {
  let best = null;
  for (const g of std.segs) {
    if (f < g.f1 || f > g.f2) continue;
    const a = g[det + "1"], b = g[det + "2"];
    if (a == null || b == null) continue;
    const t = g.f2 > g.f1 ? Math.log10(f / g.f1) / Math.log10(g.f2 / g.f1) : 0;
    const v = a + t * (b - a);
    best = best == null ? v : Math.min(best, v);
  }
  return best;
}

function emiSpec(r) {
  const h = r.harm, std = r.standard;
  const f0 = Math.min(std.fmin, h.f[0] || std.fmin) / 1.25, f1 = std.fmax * 1.08;
  let ymax = 0;
  for (const v of h.unfTotal) if (v != null && isFinite(v)) ymax = Math.max(ymax, v);
  ymax = Math.min(Math.ceil((ymax + 8) / 10) * 10, 160);
  let ylo = 0;
  for (const v of h.total) if (v != null && isFinite(v)) ylo = Math.min(ylo, v);
  const ymin = Math.max(Math.floor((ylo - 5) / 10) * 10, -40);
  const mk = limitSeries(std);
  const det = r.detector;
  const tgtSegs = mk(det === "qp" ? "qp" : det === "pk" ? "pk" : "av").map((g) => ({ x1: g.x1, x2: g.x2, y1: g.y1 == null ? null : g.y1 - r.required, y2: g.y2 == null ? null : g.y2 - r.required }));
  const series = [
    { id: "unf", label: "Unfiltered (total)", color: COL.unf, type: "markers", x: h.f, y: h.unfTotal, radius: 2 },
    { id: "pk", label: "Peak limit", color: COL.pk, type: "segments", segs: mk("pk"), dash: [2, 3], width: 1.6 },
    { id: "qp", label: "QP limit", color: COL.qp, type: "segments", segs: mk("qp"), dash: [7, 4], width: 1.8 },
    { id: "av", label: "AV limit", color: COL.av, type: "segments", segs: mk("av"), width: 2.2 },
    { id: "tgt", label: `Target (−${r.required} dB)`, color: COL.tgt, type: "segments", segs: tgtSegs, dash: [3, 3], width: 1.4 },
    { id: "dm", label: "DM (filtered)", color: COL.dm, type: "line", x: h.f, y: h.dm, width: 1.3 },
    { id: "cm", label: "CM (filtered)", color: COL.cm, type: "line", x: h.f, y: h.cm, width: 1.3 },
    { id: "total", label: "Filtered (DM+CM)", color: COL.total, type: "linemarkers", x: h.f, y: h.total, snap: true, radius: 2.6, width: 1.4 },
  ];
  const bands = std.lisn === "5uH" ? std.segs.map((g) => ({ x1: g.f1, x2: g.f2, label: g.name, color: "rgba(220,38,38,0.045)" })) : [];
  return {
    opts: { xmin: f0, xmax: f1, ymin, ymax, ylabel: "Level (dBµV)", yunit: "dBµV",
      extraTip: (x) => {
        const i = nearestIdx(h.f, x);
        const lim = limitAt(std, h.f[i], det === "qp" ? "qp" : det === "pk" ? "pk" : "av");
        if (lim == null) return [`<div style="color:#94a3b8">no limit at this frequency</div>`];
        const m = lim - h.total[i];
        return [`<div>${det.toUpperCase()} limit: <b>${lim.toFixed(1)}</b> dBµV</div>`, `<div>Margin: <b style="color:${m >= r.required ? "#86efac" : "#fca5a5"}">${m.toFixed(1)} dB</b></div>`];
      } },
    series, bands,
  };
}

function attSpec(r) {
  const a = r.atten, il = r.il;
  let ymax = 20, ymin = 0;
  for (const arr of [a.dm, a.cm, il.dm, il.cm]) for (const v of arr) if (v != null && isFinite(v)) { ymax = Math.max(ymax, v); ymin = Math.min(ymin, v); }
  ymax = Math.min(Math.ceil((ymax + 10) / 20) * 20, 180);
  ymin = Math.max(Math.floor((ymin - 5) / 10) * 10, -40);
  const P0 = P();
  return {
    opts: { xmin: a.f[0], xmax: a.f[a.f.length - 1], ymin, ymax, ylabel: "Attenuation (dB)", yunit: "dB" },
    series: [
      { id: "dmA", label: "DM in-circuit", color: COL.total, type: "line", x: a.f, y: a.dm, width: 2, snap: true },
      { id: "cmA", label: "CM in-circuit", color: COL.cm, type: "line", x: a.f, y: a.cm, width: 2 },
      { id: "dmI", label: "DM 50 Ω/50 Ω", color: COL.total, type: "line", x: il.f, y: il.dm, dash: [6, 4], width: 1.4 },
      { id: "cmI", label: "CM 50 Ω/50 Ω", color: COL.cm, type: "line", x: il.f, y: il.cm, dash: [6, 4], width: 1.4 },
      { id: "fsw", label: "fsw", color: "#94a3b8", type: "vline", x: P0.fsw, tag: "fsw", dash: [4, 4], width: 1, noHover: true },
    ],
    bands: [],
  };
}

function stabSpec(r) {
  const s = r.stab;
  let ymax = s.zinDb + 10, ymin = s.zinDb - 30;
  for (const v of s.zout) if (v != null && isFinite(v)) { ymax = Math.max(ymax, v + 6); ymin = Math.min(ymin, v - 4); }
  if (s.zoutUnd) for (const v of s.zoutUnd) if (v != null && isFinite(v)) ymax = Math.max(ymax, Math.min(v + 6, s.zinDb + 40));
  ymin = Math.max(Math.floor(ymin / 10) * 10, -80); ymax = Math.ceil(ymax / 10) * 10;
  const series = [
    { id: "zin", label: "|Zin| converter", color: COL.zin, type: "hline", y: s.zinDb, tag: `|Zin| = ${fmtEng(s.zin, "Ω")}`, width: 2, noHover: true },
    { id: "zlim", label: "Middlebrook limit", color: COL.zin, type: "hline", y: s.zinDb - P().stabMarginDb, dash: [6, 4], width: 1.3, tag: `−${P().stabMarginDb} dB`, noHover: true },
    { id: "fc", label: "Check range", color: "#94a3b8", type: "vline", x: s.fcheck, tag: "≈ converter BW", dash: [4, 4], width: 1, noHover: true },
  ];
  if (s.zoutUnd) series.push({ id: "zund", label: "|Zout| without damping", color: COL.zund, type: "line", x: s.f, y: s.zoutUnd, dash: [5, 4], width: 1.4 });
  series.push({ id: "zout", label: "|Zout| filter", color: COL.total, type: "line", x: s.f, y: s.zout, width: 2.2, snap: true });
  return { opts: { xmin: s.f[0], xmax: s.f[s.f.length - 1], ymin, ymax, ylabel: "Impedance (dBΩ)", yunit: "dBΩ" }, series, bands: [] };
}

function buildLegend(el, spec, key) {
  el.innerHTML = spec.series.filter((s) => !s.noHover || s.type === "hline").map((s) => {
    const sw = s.type === "markers" ? "sw dot" : s.dash ? "sw dash" : "sw";
    const st = s.dash ? `color:${s.color}` : `background:${s.color}`;
    return `<label><input type="checkbox" data-id="${s.id}" ${hidden[key][s.id] ? "" : "checked"}><span class="${sw}" style="${st}"></span>${esc(s.label)}</label>`;
  }).join("");
  $$("input", el).forEach((i) => i.addEventListener("change", () => { hidden[key][i.dataset.id] = !i.checked; charts[key].setVisible(i.dataset.id, i.checked); }));
}

function applySpec(key, el, spec) {
  if (!charts[key]) charts[key] = new LogChart(el, {});
  const ch = charts[key];
  ch.opts = Object.assign({ xlabel: "Frequency (Hz)" }, spec.opts);
  ch.setBands(spec.bands);
  for (const s of spec.series) s.visible = !hidden[key][s.id];
  ch.setSeries(spec.series);
}

function renderCharts() {
  const r = SIM();
  if (!r) return;
  const e = emiSpec(r);
  applySpec("emi", $("#chEmi"), e); buildLegend($("#emiLegend"), e, "emi");
  $("#emiSub").textContent = `${r.standard.name} · ${r.singleLisn ? "single artificial network" : "worst of both lines"} · harmonics of ${fmtEng(P().fsw, "Hz")} · ${r.standard.notes}`;
  const a = attSpec(r);
  applySpec("att", $("#chAtt"), a); buildLegend($("#attLegend"), a, "att");
  const s = stabSpec(r);
  applySpec("stab", $("#chStab"), s); buildLegend($("#stabLegend"), s, "stab");
  $("#stabSub").textContent = `Filter output impedance seen by the converter (including the artificial network and C_in) versus the converter's negative input impedance |Zin| = Vmin²/Pin = ${fmtEng(r.stab.zin, "Ω")}. Keep |Zout| at least ${P().stabMarginDb} dB below |Zin| up to the converter's control bandwidth.`;
}

// ---------------------------------------------------------------- BOM
const UNIT = { cap: "F", ind: "H", cmc: "H", res: "Ω" };

function reqText(c) {
  const parts = [];
  if (c.kind === "cap") {
    if (c.reqV) parts.push(`≥ ${Math.round(c.reqV)} V${c.reqVac ? "AC" : ""}`);
    if (c.class) parts.push(c.class);
    if (c.sub === "mlcc" && c.vBias && c.vRated) parts.push(`eff. ${fmtEng(effC(c), "F")} @ ${c.vBias} V`);
  }
  if (c.kind === "ind" || c.kind === "cmc") {
    if (c.reqI) parts.push(`Irms ≥ ${c.reqI.toFixed(2)} A`);
    if (c.reqIsat) parts.push(`Isat ≥ ${c.reqIsat.toFixed(2)} A`);
  }
  if (c.package) parts.push(c.package);
  return parts.join(" · ");
}
function effC(c) {
  if (c.sub !== "mlcc" || !c.vRated || !c.vBias || c.class === "C0G") return c.value;
  const x = Math.min(c.vBias / c.vRated, 1);
  let f = 1 - 0.72 * Math.pow(x, 1.3);
  if (c.value < 0.5e-6) f = 1 - 0.3 * Math.pow(x, 1.5);
  return c.value * Math.max(f, 0.2);
}

function paramInputs(c) {
  const fld = (k, label, unit) => `<label>${label} <input class="cell" data-k="${k}" value="${esc(fmtEngPlain(c[k] || 0))}"> ${unit}</label>`;
  const out = [];
  if (c.kind === "cap") { out.push(fld("esr", "ESR", "Ω"), fld("esl", "ESL", "H"), fld("vRated", "V rated", "V")); if (c.sub === "mlcc") out.push(fld("vBias", "DC bias", "V")); }
  if (c.kind === "ind") out.push(fld("dcr", "DCR", "Ω"), fld("srf", "SRF", "Hz"));
  if (c.kind === "cmc") out.push(fld("leak", "Leakage (DM)", "H"), fld("dcr", "DCR/winding", "Ω"), fld("srf", "SRF", "Hz"));
  if (c.kind === "res") out.push(`<span class="muted">Ideal resistor model</span>`);
  return `<div class="params-grid">${out.join("")}</div>`;
}

function renderBOM() {
  const c = C();
  const t = $("#bomTable");
  if (!c) { t.innerHTML = ""; return; }
  const rows = [`<tr><th></th><th>Ref</th><th>Function</th><th>Qty</th><th>Value</th><th>Selected part</th><th>Price / stock</th><th>Buy</th><th></th></tr>`];
  for (const x of c.comps) {
    const p = x.part || {};
    const exp = !!S.expanded[x.ref];
    const src = p.source ? `<span class="badge ${p.source}">${p.source === "generic" ? "no part yet" : p.source}</span>` : "";
    const partCell = p.mpn ? `<div class="mpn">${esc(p.mpn)}</div><div class="req">${esc(p.manufacturer || "")} ${src}</div>`
      : x.inBom ? `<div class="req">${src} ${esc(p.description || "")}</div>` : `<span class="req">on converter</span>`;
    const price = p.unitPrice > 0 ? `${p.unitPrice.toFixed(p.unitPrice < 1 ? 3 : 2)} ${esc(p.currency || "")}` : "";
    const stock = p.stock > 0 ? `<div class="req">${p.stock.toLocaleString()} in stock</div>` : "";
    const links = x.inBom ? `<span class="links">${p.mouserUrl ? `<a href="${esc(p.mouserUrl)}" target="_blank" rel="noopener">Mouser</a>` : ""}${p.digikeyUrl ? `<a href="${esc(p.digikeyUrl)}" target="_blank" rel="noopener">DigiKey</a>` : ""}${p.datasheetUrl ? `<a href="${esc(p.datasheetUrl)}" target="_blank" rel="noopener">Datasheet</a>` : ""}</span>` : "";
    rows.push(`<tr data-ref="${esc(x.ref)}" ${x.inBom ? "" : 'style="opacity:.75"'}>
      <td><button class="expander" data-exp="${esc(x.ref)}" title="Parasitics">${exp ? "▾" : "▸"}</button></td>
      <td class="ref">${esc(x.ref)}</td>
      <td><div class="role">${esc(x.role)}</div><div class="req">${esc(reqText(x))}</div></td>
      <td>${x.inBom ? `<input class="cell qty" data-ref="${esc(x.ref)}" data-k="qty" value="${x.qty || 1}">` : "1"}</td>
      <td class="num"><input class="cell" data-ref="${esc(x.ref)}" data-k="value" value="${esc(fmtEngPlain(x.value))}"> ${UNIT[x.kind] || ""}</td>
      <td>${partCell}</td>
      <td class="num">${price}${stock}</td>
      <td>${links}</td>
      <td>${x.inBom ? `<button class="btn small" data-find="${esc(x.ref)}">Find parts</button>` : ""}</td>
    </tr>`);
    if (exp) rows.push(`<tr class="sub" data-sub="${esc(x.ref)}"><td></td><td></td><td colspan="7">${paramInputs(x).replace(/data-k=/g, `data-ref="${esc(x.ref)}" data-k=`)}</td></tr>`);
  }
  t.innerHTML = rows.join("");
  $$("button[data-exp]", t).forEach((b) => b.addEventListener("click", () => { S.expanded[b.dataset.exp] = !S.expanded[b.dataset.exp]; renderBOM(); }));
  $$("button[data-find]", t).forEach((b) => b.addEventListener("click", () => openDrawer(b.dataset.find)));
  $$("input.cell", t).forEach((inp) => inp.addEventListener("change", () => {
    const comp = c.comps.find((q) => q.ref === inp.dataset.ref);
    let v = parseEng(inp.value);
    if (!comp || !isFinite(v) || v < 0) { inp.classList.add("bad"); return; }
    if (inp.dataset.k === "qty") v = Math.max(1, Math.round(v));
    comp[inp.dataset.k] = v;
    if (inp.dataset.k === "value" && comp.kind === "cmc" && comp.leak) { /* keep leakage */ }
    simulate();
  }));
  // cost
  let cost = 0, cur = "", n = 0;
  for (const x of c.comps) if (x.inBom && x.part && x.part.unitPrice > 0) { cost += x.part.unitPrice * (x.qty || 1); cur = x.part.currency; n++; }
  $("#bomCost").textContent = n ? `Priced lines: ${n} · total ${cost.toFixed(2)} ${cur} (qty 1)` : "";
}

// ---------------------------------------------------------------- part search drawer
let drawerRef = null;
function openDrawer(ref) {
  drawerRef = ref;
  const c = C().comps.find((q) => q.ref === ref);
  $("#drTitle").textContent = `Find parts – ${ref}`;
  $("#drSub").textContent = `${c.role} · ${fmtEng(c.value, UNIT[c.kind])} · ${reqText(c)}`;
  $("#drKeyword").value = "";
  const st = S.init.settings;
  $("#srcMouser").disabled = !st.mouser; $("#srcMouser").checked = !!st.mouser;
  $("#srcDigikey").disabled = !st.digikey; $("#srcDigikey").checked = !!st.digikey;
  $("#drawer").hidden = false;
  searchParts();
}
async function searchParts() {
  const c = C().comps.find((q) => q.ref === drawerRef);
  if (!c) return;
  const sources = [];
  if ($("#srcLib").checked) sources.push("library");
  if ($("#srcMouser").checked && !$("#srcMouser").disabled) sources.push("mouser");
  if ($("#srcDigikey").checked && !$("#srcDigikey").disabled) sources.push("digikey");
  const msg = $("#drMsg");
  msg.innerHTML = "Searching…";
  $("#drTable").innerHTML = "";
  try {
    const r = await api("/api/parts", { comp: c, keyword: $("#drKeyword").value, sources });
    if (!$("#drKeyword").value) $("#drKeyword").value = r.keyword;
    const bits = [];
    for (const [k, v] of Object.entries(r.counts || {})) bits.push(`${k}: ${v}`);
    for (const [k, v] of Object.entries(r.errors || {})) bits.push(`<span class="err">${esc(k)}: ${esc(v)}</span>`);
    const st = S.init.settings;
    if (!st.mouser && !st.digikey) bits.push(`Add Mouser / DigiKey API keys in <a href="#" id="drKeys">Settings</a> for live price &amp; stock – or use the search links: <a href="${esc(c.part && c.part.mouserUrl || "#")}" target="_blank">Mouser</a> · <a href="${esc(c.part && c.part.digikeyUrl || "#")}" target="_blank">DigiKey</a>`);
    msg.innerHTML = bits.join(" · ") || "No results";
    const k = $("#drKeys"); if (k) k.addEventListener("click", (e) => { e.preventDefault(); openSettings(); });
    renderResults(c, r.results || []);
  } catch (e) { msg.innerHTML = `<span class="err">${esc(e.message)}</span>`; }
}
function renderResults(c, res) {
  const unit = UNIT[c.kind];
  const rows = [`<tr><th>Source</th><th>Part</th><th>Description</th><th>Value</th><th>Rating</th><th>DCR/ESR</th><th>Price</th><th>Stock</th><th>Fit</th><th></th></tr>`];
  res.forEach((x, i) => {
    const rating = c.kind === "cap" ? (x.vRated ? `${x.vRated} V` : "") : (x.iRated ? `${x.iRated} A` : "") + (x.iSat ? ` / sat ${x.iSat} A` : "");
    const fit = { ok: "fits", value: "value differs", rating: "rating too low", unknown: "check" }[x.match] || x.match;
    const link = x.source === "digikey" ? x.digikeyUrl : x.source === "mouser" ? x.mouserUrl : x.mouserUrl;
    rows.push(`<tr>
      <td><span class="badge ${x.source}">${x.source}</span></td>
      <td><div class="mpn"><a href="${esc(link || "#")}" target="_blank" rel="noopener">${esc(x.mpn)}</a></div><div class="req">${esc(x.manufacturer)}</div></td>
      <td class="desc">${esc(x.description)}</td>
      <td class="num">${x.value ? fmtEng(x.value, unit) : "–"}</td>
      <td class="num">${esc(rating)}</td>
      <td class="num">${x.dcr ? fmtEng(x.dcr, "Ω") : x.esr ? fmtEng(x.esr, "Ω") : "–"}</td>
      <td class="num">${x.unitPrice ? x.unitPrice.toFixed(x.unitPrice < 1 ? 3 : 2) + " " + esc(x.currency || "") : "–"}</td>
      <td class="num">${x.source === "library" ? "–" : (x.stock || 0).toLocaleString()}</td>
      <td><span class="badge ${x.match}">${fit}</span></td>
      <td><button class="btn small primary" data-use="${i}">Use</button></td></tr>`);
  });
  if (res.length === 0) rows.push(`<tr><td colspan="10" class="muted">No matching parts. Edit the keywords (e.g. value, voltage, package) and search again.</td></tr>`);
  const t = $("#drTable");
  t.innerHTML = rows.join("");
  $$("button[data-use]", t).forEach((b) => b.addEventListener("click", async () => {
    const cand = res[+b.dataset.use];
    try {
      const r = await api("/api/applypart", { comp: c, cand });
      const comps = C().comps;
      const i = comps.findIndex((q) => q.ref === c.ref);
      comps[i] = r.comp;
      $("#drawer").hidden = true;
      toast(`${c.ref} → ${cand.mpn}`);
      simulate();
    } catch (e) { toast(e.message, 4000); }
  }));
}

// ---------------------------------------------------------------- log / spice
function renderLog() {
  const lines = S.log[S.mode] || [];
  $("#logList").innerHTML = lines.map((l) => `<li class="${/^WARNING/.test(l) ? "w" : ""}">${esc(l)}</li>`).join("") || `<li class="muted">Run “Design filter” to synthesize a filter.</li>`;
  const r = SIM();
  if (!r) return;
  const rows = [`<tr><th>Operating point</th><th>Vin</th><th>Iin (avg)</th><th>Duty</th><th>Current waveform</th><th>Switch-node swing</th></tr>`];
  for (const o of r.ops) rows.push(`<tr><td>${esc(o.label)}</td><td class="num">${fmtEng(o.vin, "V")}</td><td class="num">${fmtEng(o.iin, "A")}</td><td class="num">${(o.d * 100).toFixed(1)} %</td><td>${o.shape === "triangle" ? `triangle, ${fmtEng(o.ripp, "A")} p-p` : `pulsed, ${fmtEng(o.ipk, "A")} peak`}</td><td class="num">${fmtEng(o.vsw, "V")}</td></tr>`);
  $("#opsTable").innerHTML = rows.join("");
}
async function loadSpice() {
  if (!C()) return;
  if (!S.spiceStale && $("#spiceText").textContent) return;
  try { $("#spiceText").textContent = await api("/api/export/spice", { params: P(), circuit: C() }); S.spiceStale = false; }
  catch (e) { $("#spiceText").textContent = "Error: " + e.message; }
}

// ---------------------------------------------------------------- files
async function saveFile(name, content, mime, desc) {
  const blob = content instanceof Blob ? content : new Blob([content], { type: mime });
  if (window.showSaveFilePicker) {
    try {
      const ext = "." + name.split(".").pop();
      const h = await window.showSaveFilePicker({ suggestedName: name, types: [{ description: desc || name, accept: { [mime]: [ext] } }] });
      const w = await h.createWritable(); await w.write(blob); await w.close();
      toast("Saved " + h.name);
      return;
    } catch (e) { if (e.name === "AbortError") return; }
  }
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob); a.download = name;
  document.body.appendChild(a); a.click(); a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 4000);
}
function baseName() { return (S.mode === "dc" ? "dc" : "ac") + "-emi-filter"; }

async function doExport(kind) {
  $("#exportMenu").hidden = true;
  if (!C() && kind !== "open") { toast("Design a filter first"); return; }
  try {
    if (kind === "bom") saveFile(baseName() + "-bom.csv", await api("/api/export/bom", { params: P(), circuit: C() }), "text/csv", "CSV file");
    if (kind === "spice") saveFile(baseName() + ".cir", await api("/api/export/spice", { params: P(), circuit: C() }), "text/plain", "SPICE netlist");
    if (kind === "save") {
      const proj = { app: "emi-filter-designer", version: S.init.version, mode: S.mode, params: S.params, circuit: S.circuit, log: S.log };
      saveFile(baseName() + ".emif.json", JSON.stringify(proj, null, 1), "application/json", "EMI filter project");
    }
    if (kind === "open") $("#fileOpen").click();
    if (kind === "report") saveFile(baseName() + "-report.html", await buildReport(), "text/html", "HTML report");
  } catch (e) { toast("Export failed: " + e.message, 5000); }
}

function offscreenChart(spec) {
  const d = document.createElement("div");
  d.style.cssText = "position:fixed;left:-12000px;top:0;width:1000px;height:430px";
  document.body.appendChild(d);
  const ch = new LogChart(d, Object.assign({ xlabel: "Frequency (Hz)" }, spec.opts));
  ch.setBands(spec.bands);
  for (const s of spec.series) s.visible = true;
  ch.series = spec.series; ch.render();
  const url = ch.toDataURL();
  d.remove();
  return url;
}

async function buildReport() {
  const r = SIM(), c = C(), p = P();
  const bom = await api("/api/export/bom", { params: p, circuit: c });
  const lines = bom.replace(/^\ufeff/, "").trim().split(/\r?\n/).map(parseCSVLine);
  const head = lines.shift();
  const keep = [0, 1, 2, 3, 4, 5, 7, 8];
  const bomHtml = `<table><tr>${keep.map((i) => `<th>${esc(head[i])}</th>`).join("")}</tr>${lines.map((l) => `<tr>${keep.map((i) => `<td>${esc(l[i])}</td>`).join("")}</tr>`).join("")}</table>`;
  const prm = FIELDS.filter((f) => f.key && fieldVisible(f) && f.type !== "check").map((f) => {
    let v = p[f.key];
    if (f.type === "select") v = (f.options().find((o) => o[0] === String(v)) || [0, v])[1];
    else v = (f.eng ? fmtEng(v, f.unit) : displayVal(f, v) + " " + (f.unit || ""));
    return `<tr><td>${esc(f.label)}</td><td>${esc(v)}</td></tr>`;
  }).join("");
  const svg = $("#schematic").innerHTML;
  const emi = offscreenChart(emiSpec(r)), att = offscreenChart(attSpec(r)), stab = offscreenChart(stabSpec(r));
  const pass = r.margin >= r.required;
  return `<!doctype html><html><head><meta charset="utf-8"><title>EMI filter design report</title>
<style>body{font-family:Segoe UI,system-ui,sans-serif;color:#0f172a;max-width:1060px;margin:24px auto;padding:0 20px;font-size:14px}
h1{font-size:22px;margin:0 0 4px}h2{font-size:16px;margin:26px 0 8px;border-bottom:1px solid #e2e8f0;padding-bottom:4px}
table{border-collapse:collapse;width:100%;font-size:12.5px}td,th{border-bottom:1px solid #e2e8f0;padding:5px 7px;text-align:left;vertical-align:top}
th{background:#f8fafc;font-size:11.5px;text-transform:uppercase;color:#64748b}.kpi{display:flex;gap:12px;flex-wrap:wrap}
.k{border:1px solid #e2e8f0;border-radius:8px;padding:8px 12px;min-width:170px}.k b{display:block;font-size:20px}
.ok{color:#15803d}.bad{color:#b91c1c}img{width:100%;border:1px solid #e2e8f0;border-radius:6px}svg{max-width:100%;height:auto}
.muted{color:#64748b;font-size:12px}.two{display:grid;grid-template-columns:1fr 1fr;gap:18px}@media print{h2{break-after:avoid}img,svg{break-inside:avoid}}</style></head><body>
<h1>EMI input filter – design report</h1>
<div class="muted">${esc(r.standard.name)} · ${S.mode === "dc" ? "DC input" : "AC mains"} · ${esc(c.description)} · generated ${new Date().toLocaleString()} by EMI Filter Designer ${esc(S.init.version)}</div>
<h2>Summary</h2><div class="kpi">
<div class="k">Emission margin<b class="${pass ? "ok" : "bad"}">${fmtDb(r.margin)} ${pass ? "PASS" : "FAIL"}</b><span class="muted">target ≥ ${r.required} dB, worst at ${fmtEng(r.marginF, "Hz")}</span></div>
<div class="k">DM / CM margin<b>${fmtDb(r.marginDm)} / ${fmtDb(r.marginCm)}</b><span class="muted">unfiltered ${fmtDb(r.unfMargin)}</span></div>
<div class="k">Stability (Middlebrook)<b class="${r.stab.pass ? "ok" : "bad"}">${fmtDb(r.stab.marginDb)}</b><span class="muted">|Zin| = ${fmtEng(r.stab.zin, "Ω")}</span></div>
<div class="k">Drop / loss<b>${fmtEng(r.dropV, "V")}</b><span class="muted">${fmtEng(r.lossW, "W")} at ${fmtEng(r.irms, "A")}</span></div></div>
${r.warnings && r.warnings.length ? `<ul>${r.warnings.map((w) => `<li class="bad">${esc(w)}</li>`).join("")}</ul>` : ""}
<h2>Schematic</h2>${svg}
${(c.notes || []).map((n) => `<p class="muted">${esc(n)}</p>`).join("")}
<h2>Bill of materials</h2>${bomHtml}
<h2>Conducted emissions</h2><img src="${emi}" alt="Emissions">
<div class="two"><div><h2>Attenuation</h2><img src="${att}" alt="Attenuation"></div><div><h2>Stability</h2><img src="${stab}" alt="Stability"></div></div>
<h2>Design inputs</h2><table>${prm}</table>
<h2>Design log</h2><ul>${(S.log[S.mode] || []).map((l) => `<li>${esc(l)}</li>`).join("")}</ul>
<p class="muted">Model: nodal analysis of every switching harmonic with the artificial network(s), component parasitics (ESR/ESL, DCR/SRF, CM-choke leakage) and MLCC DC-bias derating. DM and CM contributions summed in magnitude (worst case). Layout coupling is not modelled – verify with a pre-compliance measurement.</p>
</body></html>`;
}
function parseCSVLine(l) {
  const out = []; let cur = "", q = false;
  for (let i = 0; i < l.length; i++) {
    const ch = l[i];
    if (q) { if (ch === '"') { if (l[i + 1] === '"') { cur += '"'; i++; } else q = false; } else cur += ch; }
    else if (ch === '"') q = true; else if (ch === ",") { out.push(cur); cur = ""; } else cur += ch;
  }
  out.push(cur); return out;
}

// ---------------------------------------------------------------- settings
function openSettings() {
  const st = S.init.settings;
  $("#cfgPath").textContent = st.configDir;
  $("#kMouser").value = st.mouserKey || ""; $("#kDkId").value = st.digikeyClientId || ""; $("#kDkSecret").value = st.digikeySecret || "";
  $("#kDkSite").value = st.digikeySite || "US"; $("#kDkCur").value = st.digikeyCurrency || "USD"; $("#kStock").checked = !!st.inStockOnly;
  $("#keyTest").textContent = "";
  $("#proxyRow").hidden = !st.static; $("#staticNote").hidden = !st.static; $("#kProxy").value = st.proxy || "";
  $("#settingsModal").hidden = false;
}
async function saveSettings() {
  const body = { mouserKey: $("#kMouser").value, digikeyClientId: $("#kDkId").value, digikeySecret: $("#kDkSecret").value,
    digikeySite: $("#kDkSite").value.toUpperCase(), digikeyCurrency: $("#kDkCur").value.toUpperCase(), inStockOnly: $("#kStock").checked, proxy: $("#kProxy").value };
  S.init.settings = await api("/api/settings", body);
  return S.init.settings;
}

// ---------------------------------------------------------------- tabs / mode
function switchTab(t) {
  $$(".tabs button").forEach((b) => b.classList.toggle("on", b.dataset.tab === t));
  $$(".panel").forEach((p) => p.classList.toggle("on", p.id === "p-" + t));
  for (const k in charts) charts[k].render();
  if (t === "spice") loadSpice();
}
function setMode(m) {
  if (S.mode === m) return;
  S.mode = m;
  $$("#modeSeg button").forEach((b) => b.classList.toggle("on", b.dataset.mode === m));
  renderForm();
  S.spiceStale = true;
  if (!C()) { design(); return; }
  renderAll();
}

// ---------------------------------------------------------------- boot
async function boot() {
  S.init = await api("/api/init");
  S.params.dc = S.init.defaults.dc; S.params.ac = S.init.defaults.ac;
  renderForm();
  $$("#modeSeg button").forEach((b) => b.addEventListener("click", () => setMode(b.dataset.mode)));
  $$(".tabs button").forEach((b) => b.addEventListener("click", () => switchTab(b.dataset.tab)));
  $("#btnDesign").addEventListener("click", design);
  $("#btnSim").addEventListener("click", simulate);
  $("#btnExport").addEventListener("click", (e) => { e.stopPropagation(); $("#exportMenu").hidden = !$("#exportMenu").hidden; });
  document.addEventListener("click", () => ($("#exportMenu").hidden = true));
  $$("#exportMenu button").forEach((b) => b.addEventListener("click", () => doExport(b.dataset.exp)));
  $("#btnBomCsv").addEventListener("click", () => doExport("bom"));
  $("#btnSaveSpice").addEventListener("click", () => doExport("spice"));
  $("#btnCopySpice").addEventListener("click", async () => { await navigator.clipboard.writeText($("#spiceText").textContent); toast("Netlist copied"); });
  $("#drClose").addEventListener("click", () => ($("#drawer").hidden = true));
  $("#drGo").addEventListener("click", searchParts);
  $("#drKeyword").addEventListener("keydown", (e) => { if (e.key === "Enter") searchParts(); });
  $("#btnSettings").addEventListener("click", openSettings);
  $$("[data-close]").forEach((b) => b.addEventListener("click", () => ($("#settingsModal").hidden = true)));
  $("#btnSaveKeys").addEventListener("click", async () => { try { await saveSettings(); $("#settingsModal").hidden = true; toast("Settings saved"); } catch (e) { toast(e.message, 4000); } });
  $("#btnTestKeys").addEventListener("click", async () => {
    $("#keyTest").textContent = "Testing…";
    try { await saveSettings(); const r = await api("/api/testkeys"); $("#keyTest").textContent = `Mouser: ${r.mouser} · DigiKey: ${r.digikey}`; }
    catch (e) { $("#keyTest").textContent = e.message; }
  });
  $("#fileOpen").addEventListener("change", async (e) => {
    const f = e.target.files[0]; if (!f) return;
    try {
      const j = JSON.parse(await f.text());
      if (j.app !== "emi-filter-designer") throw new Error("not an EMI Filter Designer project");
      S.params = Object.assign(S.params, j.params); S.circuit = Object.assign({ dc: null, ac: null }, j.circuit); S.log = j.log || { dc: [], ac: [] };
      S.sim = { dc: null, ac: null };
      S.mode = j.mode === "ac" ? "ac" : "dc";
      $$("#modeSeg button").forEach((b) => b.classList.toggle("on", b.dataset.mode === S.mode));
      renderForm(); simulate(); toast("Opened " + f.name);
    } catch (err) { toast("Could not open: " + err.message, 5000); }
    e.target.value = "";
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); design(); }
    if (e.key === "Escape") { $("#drawer").hidden = true; $("#settingsModal").hidden = true; }
  });
  // keep-alive for the local server (it exits when the window is closed)
  if (!window.STATIC_API) {
    const ping = () => fetch("/api/ping", { method: "POST" }).catch(() => {});
    ping(); setInterval(ping, 5000);
    window.addEventListener("pagehide", () => navigator.sendBeacon("/api/quit"));
  }
  await design();
}
boot().catch((e) => { document.body.innerHTML = `<pre style="padding:20px;color:#b91c1c">Failed to start: ${esc(e.message)}</pre>`; });
