/* Minimal log-frequency chart on <canvas> (no external libraries). */
"use strict";

function fmtFreq(f) {
  if (f >= 1e9) return +(f / 1e9).toPrecision(3) + "G";
  if (f >= 1e6) return +(f / 1e6).toPrecision(3) + "M";
  if (f >= 1e3) return +(f / 1e3).toPrecision(3) + "k";
  return +f.toPrecision(3) + "";
}

class LogChart {
  constructor(el, opts) {
    this.el = el;
    this.opts = Object.assign({ xmin: 1e4, xmax: 1e8, ymin: 0, ymax: 100, ylabel: "", xlabel: "Frequency (Hz)", yunit: "" }, opts || {});
    this.series = [];
    this.bands = [];
    this.canvas = document.createElement("canvas");
    el.appendChild(this.canvas);
    this.ctx = this.canvas.getContext("2d");
    this.hoverX = null;
    this.pad = { l: 58, r: 18, t: 14, b: 42 };
    this.tip = document.getElementById("tip");
    new ResizeObserver(() => this.render()).observe(el);
    this.canvas.addEventListener("mousemove", (e) => this.onMove(e));
    this.canvas.addEventListener("mouseleave", () => { this.hoverX = null; this.tip.hidden = true; this.render(); });
  }
  setOpts(o) { Object.assign(this.opts, o); }
  setSeries(s) { this.series = s; this.render(); }
  setBands(b) { this.bands = b || []; }
  setVisible(id, v) { for (const s of this.series) if (s.id === id) s.visible = v; this.render(); }

  px(x) { const o = this.opts, W = this.w - this.pad.l - this.pad.r;
    return this.pad.l + (Math.log10(x) - Math.log10(o.xmin)) / (Math.log10(o.xmax) - Math.log10(o.xmin)) * W; }
  py(y) { const o = this.opts, H = this.h - this.pad.t - this.pad.b;
    return this.pad.t + (1 - (y - o.ymin) / (o.ymax - o.ymin)) * H; }
  xv(px) { const o = this.opts, W = this.w - this.pad.l - this.pad.r;
    return Math.pow(10, Math.log10(o.xmin) + (px - this.pad.l) / W * (Math.log10(o.xmax) - Math.log10(o.xmin))); }

  render() {
    const r = this.el.getBoundingClientRect();
    if (r.width < 10 || r.height < 10) return;
    const dpr = window.devicePixelRatio || 1;
    this.w = r.width; this.h = r.height;
    this.canvas.width = Math.round(r.width * dpr); this.canvas.height = Math.round(r.height * dpr);
    const c = this.ctx; c.setTransform(dpr, 0, 0, dpr, 0, 0);
    c.clearRect(0, 0, this.w, this.h);
    const o = this.opts, P = this.pad;
    const x0 = P.l, x1 = this.w - P.r, y0 = P.t, y1 = this.h - P.b;
    c.font = "11.5px 'Segoe UI', system-ui, sans-serif";

    // bands
    let lastLbl = -1e9;
    for (const b of this.bands) {
      const a = Math.max(this.px(b.x1), x0), z = Math.min(this.px(b.x2), x1);
      if (z <= a) continue;
      c.fillStyle = b.color || "rgba(37,99,235,0.05)";
      c.fillRect(a, y0, z - a, y1 - y0);
      const tw = b.label ? c.measureText(b.label).width : 0;
      const cxl = (a + z) / 2;
      if (b.label && z - a > tw * 0.6 && cxl - tw / 2 > lastLbl + 4) {
        c.fillStyle = "#94a3b8"; c.textAlign = "center"; c.textBaseline = "top";
        c.fillText(b.label, cxl, y0 + 3);
        lastLbl = cxl + tw / 2;
      }
    }
    // grid
    c.strokeStyle = "#eef2f7"; c.lineWidth = 1;
    const d0 = Math.floor(Math.log10(o.xmin)), d1 = Math.ceil(Math.log10(o.xmax));
    const span = Math.log10(o.xmax / o.xmin);
    c.textAlign = "center"; c.textBaseline = "top"; c.fillStyle = "#64748b";
    for (let d = d0; d <= d1; d++) {
      for (let m = 1; m <= 9; m++) {
        const f = m * Math.pow(10, d);
        if (f < o.xmin * 0.999 || f > o.xmax * 1.001) continue;
        const X = Math.round(this.px(f)) + 0.5;
        c.strokeStyle = m === 1 ? "#dde3ea" : "#f1f4f8";
        c.beginPath(); c.moveTo(X, y0); c.lineTo(X, y1); c.stroke();
        if (m === 1 || (span <= 3.2 && (m === 2 || m === 5))) c.fillText(fmtFreq(f), X, y1 + 6);
      }
    }
    const ystep = niceStep((o.ymax - o.ymin) / 9);
    c.textAlign = "right"; c.textBaseline = "middle";
    for (let y = Math.ceil(o.ymin / ystep) * ystep; y <= o.ymax + 1e-9; y += ystep) {
      const Y = Math.round(this.py(y)) + 0.5;
      c.strokeStyle = "#e8edf3"; c.beginPath(); c.moveTo(x0, Y); c.lineTo(x1, Y); c.stroke();
      c.fillStyle = "#64748b"; c.fillText(+y.toFixed(2), x0 - 7, Y);
    }
    c.strokeStyle = "#94a3b8"; c.strokeRect(x0 + 0.5, y0 + 0.5, x1 - x0, y1 - y0);
    // axis labels
    c.fillStyle = "#475569"; c.textAlign = "center"; c.textBaseline = "bottom";
    c.fillText(o.xlabel, (x0 + x1) / 2, this.h - 3);
    c.save(); c.translate(13, (y0 + y1) / 2); c.rotate(-Math.PI / 2); c.textBaseline = "middle"; c.fillText(o.ylabel, 0, 0); c.restore();

    // series
    c.save(); c.beginPath(); c.rect(x0, y0, x1 - x0, y1 - y0); c.clip();
    for (const s of this.series) {
      if (s.visible === false) continue;
      c.strokeStyle = s.color; c.fillStyle = s.color; c.lineWidth = s.width || 1.6;
      c.setLineDash(s.dash || []);
      if (s.type === "segments") {
        for (const g of s.segs) {
          if (g.y1 == null || g.y2 == null) continue;
          c.beginPath(); c.moveTo(this.px(g.x1), this.py(g.y1)); c.lineTo(this.px(g.x2), this.py(g.y2)); c.stroke();
        }
      } else if (s.type === "hline") {
        const Y = this.py(s.y); c.beginPath(); c.moveTo(x0, Y); c.lineTo(x1, Y); c.stroke();
        if (s.tag) { c.setLineDash([]); c.textAlign = "right"; c.textBaseline = "bottom"; c.fillText(s.tag, x1 - 5, Y - 3); }
      } else if (s.type === "vline") {
        const X = this.px(s.x); c.beginPath(); c.moveTo(X, y0); c.lineTo(X, y1); c.stroke();
        if (s.tag) { c.setLineDash([]); c.textAlign = "left"; c.textBaseline = "top"; c.fillText(s.tag, X + 4, y0 + 16); }
      } else {
        const line = s.type === "line" || s.type === "linemarkers";
        if (line) {
          c.beginPath(); let pen = false;
          for (let i = 0; i < s.x.length; i++) {
            const y = s.y[i];
            if (y == null || !isFinite(y)) { pen = false; continue; }
            const X = this.px(s.x[i]), Y = this.py(Math.max(Math.min(y, o.ymax + 50), o.ymin - 50));
            if (!pen) { c.moveTo(X, Y); pen = true; } else c.lineTo(X, Y);
          }
          c.globalAlpha = s.type === "linemarkers" ? 0.45 : 1;
          c.stroke(); c.globalAlpha = 1;
        }
        if (s.type === "markers" || s.type === "linemarkers") {
          c.setLineDash([]);
          const rr = s.radius || 2.4;
          const n = s.x.length, dense = n > 400;
          for (let i = 0; i < n; i++) {
            const y = s.y[i]; if (y == null || !isFinite(y)) continue;
            const X = this.px(s.x[i]), Y = this.py(y);
            if (dense) { c.fillRect(X - 1.2, Y - 1.2, 2.4, 2.4); continue; }
            c.beginPath(); c.arc(X, Y, rr, 0, 2 * Math.PI); c.fill();
          }
        }
      }
    }
    c.setLineDash([]);
    // hover crosshair
    if (this.hoverX != null) {
      const X = this.px(this.hoverX);
      c.strokeStyle = "rgba(15,23,42,.35)"; c.lineWidth = 1; c.beginPath(); c.moveTo(X, y0); c.lineTo(X, y1); c.stroke();
      for (const h of this.hoverPts || []) {
        c.fillStyle = "#fff"; c.strokeStyle = h.color; c.lineWidth = 2;
        c.beginPath(); c.arc(this.px(h.x), this.py(h.y), 4, 0, 2 * Math.PI); c.fill(); c.stroke();
      }
    }
    c.restore();
  }

  onMove(e) {
    const r = this.canvas.getBoundingClientRect();
    const mx = e.clientX - r.left, my = e.clientY - r.top;
    if (mx < this.pad.l || mx > this.w - this.pad.r || my < this.pad.t || my > this.h - this.pad.b) {
      this.hoverX = null; this.tip.hidden = true; this.render(); return;
    }
    let xv = this.xv(mx);
    // snap to the nearest point of the primary (snap) series
    const snap = this.series.find((s) => s.snap && s.visible !== false && s.x && s.x.length);
    if (snap) {
      const i = nearestIdx(snap.x, xv);
      if (Math.abs(this.px(snap.x[i]) - mx) < 40) xv = snap.x[i];
    }
    this.hoverX = xv;
    const pts = [];
    const rows = [];
    for (const s of this.series) {
      if (s.visible === false || !s.x || !s.x.length || s.noHover) continue;
      const i = nearestIdx(s.x, xv);
      const y = s.y[i];
      if (y == null || !isFinite(y)) continue;
      if (Math.abs(Math.log10(s.x[i] / xv)) > 0.03) continue;
      pts.push({ x: s.x[i], y, color: s.color });
      rows.push(`<div><span class="sw" style="background:${s.color}"></span>${s.label}: <b>${y.toFixed(1)}</b> ${this.opts.yunit}</div>`);
    }
    if (this.opts.extraTip) rows.push(...this.opts.extraTip(xv));
    this.hoverPts = pts;
    this.tip.innerHTML = `<div><b>${fmtFreq(xv)}Hz</b></div>` + rows.join("");
    this.tip.hidden = false;
    const tw = this.tip.offsetWidth, th = this.tip.offsetHeight;
    let tx = e.clientX + 14, ty = e.clientY + 14;
    if (tx + tw > window.innerWidth - 8) tx = e.clientX - tw - 14;
    if (ty + th > window.innerHeight - 8) ty = e.clientY - th - 14;
    this.tip.style.left = tx + "px"; this.tip.style.top = ty + "px";
    this.render();
  }

  toDataURL() { return this.canvas.toDataURL("image/png"); }
}

function niceStep(raw) {
  const p = Math.pow(10, Math.floor(Math.log10(raw)));
  const m = raw / p;
  return (m <= 1 ? 1 : m <= 2 ? 2 : m <= 5 ? 5 : 10) * p;
}

function nearestIdx(arr, x) {
  let lo = 0, hi = arr.length - 1;
  while (hi - lo > 1) { const mid = (lo + hi) >> 1; if (arr[mid] < x) lo = mid; else hi = mid; }
  return Math.abs(Math.log(arr[lo] / x)) <= Math.abs(Math.log(arr[hi] / x)) ? lo : hi;
}
