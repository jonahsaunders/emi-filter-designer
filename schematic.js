/* Ladder schematic renderer (SVG) for the EMI filter circuit. */
"use strict";

const SC = { yT: 78, yB: 262, stroke: "#1e293b", accent: "#2563eb", muted: "#64748b" };

function escSvg(s) { return String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c])); }

function compLabel(c) {
  const unit = { cap: "F", ind: "H", cmc: "H", res: "Ω" }[c.kind] || "";
  const v = fmtEng(c.value, unit);
  return (c.qty > 1 ? c.qty + "× " : "") + v;
}

function drawSchematic(el, circ, mode, singleLisn, onClick) {
  if (!circ) { el.innerHTML = ""; return; }
  const byRef = {};
  for (const c of circ.comps) byRef[c.ref] = c;
  const out = [];
  let x = 24;
  const yT = SC.yT, yB = SC.yB, yM = (yT + yB) / 2;
  const topLbl = mode === "ac" ? "L" : "+";
  const botLbl = mode === "ac" ? "N" : "−";
  const line = (x1, y1, x2, y2, cls) => `<line x1="${x1}" y1="${y1}" x2="${x2}" y2="${y2}" class="${cls || "w"}"/>`;
  const dot = (cx, cy) => `<circle cx="${cx}" cy="${cy}" r="3.2" class="dot"/>`;
  const text = (tx, ty, s, cls, anchor) => `<text x="${tx}" y="${ty}" class="${cls || "t"}" text-anchor="${anchor || "start"}">${escSvg(s)}</text>`;
  const rails = (x1, x2, top = true, bot = true) => (top ? line(x1, yT, x2, yT) : "") + (bot ? line(x1, yB, x2, yB) : "");

  // vertical capacitor between ya and yb at x
  function capV(cx, ya, yb, c) {
    const m = (ya + yb) / 2;
    const pol = c && (c.sub === "elec" || c.sub === "polymer");
    let s = line(cx, ya, cx, m - 5) + line(cx, m + 5, cx, yb);
    s += `<line x1="${cx - 15}" y1="${m - 5}" x2="${cx + 15}" y2="${m - 5}" class="sym"/>`;
    if (pol) {
      s += `<path d="M${cx - 15} ${m + 9} Q${cx} ${m + 1} ${cx + 15} ${m + 9}" class="sym" fill="none"/>`;
      s += text(cx - 22, m - 9, "+", "t small");
    } else s += `<line x1="${cx - 15}" y1="${m + 5}" x2="${cx + 15}" y2="${m + 5}" class="sym"/>`;
    return s;
  }
  function resV(cx, ya, yb) {
    const m = (ya + yb) / 2;
    return line(cx, ya, cx, m - 17) + line(cx, m + 17, cx, yb) + `<rect x="${cx - 7}" y="${m - 17}" width="14" height="34" class="sym" fill="#fff"/>`;
  }
  function indH(xa, xb, y, flip) {
    const n = 4, w = 14, tot = n * w, s0 = (xa + xb - tot) / 2;
    let d = `M${xa} ${y} L${s0} ${y}`;
    for (let i = 0; i < n; i++) d += ` a${w / 2} ${w / 2} 0 0 ${flip ? 0 : 1} ${w} 0`;
    d += ` L${xb} ${y}`;
    return `<path d="${d}" class="sym" fill="none"/>`;
  }
  function lw(c) {
    if (!c) return 60;
    const a = c.ref.length * 8, b = compLabel(c).length * 7.2, m = c.part && c.part.mpn ? c.part.mpn.length * 6.1 : 0;
    return Math.max(a, b, m);
  }
  function group(ref, inner) { return `<g class="schem-part" data-ref="${escSvg(ref)}">${inner}</g>`; }
  function lbl(cx, cy, c, anchor) {
    if (!c) return "";
    return text(cx, cy, c.ref, "t ref", anchor) + text(cx, cy + 15, compLabel(c), "t val", anchor) +
      (c.part && c.part.mpn ? text(cx, cy + 29, c.part.mpn, "t mpn", anchor) : "");
  }

  for (const st of circ.layout) {
    if (st.type === "lisn") {
      const bw = 92;
      out.push(`<rect x="${x}" y="${yT - 38}" width="${bw}" height="${yB - yT + 76}" rx="8" class="box"/>`);
      out.push(text(x + bw / 2, yM - 10, singleLisn ? "LISN" : "LISN ×2", "t boxt", "middle"));
      out.push(text(x + bw / 2, yM + 8, mode === "ac" ? "50 µH / 50 Ω" : (circ._lisn || "AN"), "t small", "middle"));
      out.push(text(x + bw / 2, yM + 24, mode === "ac" ? "mains" : "supply", "t small", "middle"));
      out.push(text(x + bw + 6, yT - 8, topLbl, "t term"));
      out.push(text(x + bw + 6, yB + 20, botLbl, "t term"));
      x += bw;
      out.push(rails(x, x + 26));
      x += 26;
      if (singleLisn) {
        out.push(line(x - 12, yB, x - 12, yB + 22) + earth(x - 12, yB + 22));
      }
      continue;
    }
    if (st.type === "shunt") {
      for (const r of st.refs) {
        const c = byRef[r]; const w = Math.max(92, 30 + 22 + lw(c) + 14), cx = x + 30;
        out.push(rails(x, x + w));
        out.push(dot(cx, yT) + dot(cx, yB));
        out.push(group(r, capV(cx, yT, yB, c) + lbl(cx + 22, yM - 14, c)));
        x += w;
      }
      continue;
    }
    if (st.type === "shunt_series") {
      const w = Math.max(100, 30 + 20 + Math.max(...st.refs.map((r) => lw(byRef[r]))) + 14), cx = x + 30;
      out.push(rails(x, x + w) + dot(cx, yT) + dot(cx, yB));
      const n = st.refs.length;
      const seg = (yB - yT) / n;
      st.refs.forEach((r, i) => {
        const c = byRef[r];
        const ya = yT + i * seg, yb = yT + (i + 1) * seg;
        const sym = c && c.kind === "res" ? resV(cx, ya, yb) : capV(cx, ya, yb, c);
        out.push(group(r, sym + lbl(cx + 20, (ya + yb) / 2 - 10, c)));
      });
      x += w;
      continue;
    }
    if (st.type === "series_top" || st.type === "series_bot" || st.type === "series_both") {
      const refs = st.refs;
      const w = Math.max(124, ...refs.map((r) => lw(byRef[r]) + 20));
      if (st.type !== "series_bot") {
        const c = byRef[refs[0]];
        out.push(group(refs[0], indH(x, x + w, yT) + lbl(x + w / 2, yT - 44, c, "middle")));
      } else out.push(line(x, yT, x + w, yT));
      if (st.type === "series_both") {
        const c = byRef[refs[1]];
        out.push(group(refs[1], indH(x, x + w, yB, true) + lbl(x + w / 2, yB + 26, c, "middle")));
      } else if (st.type === "series_bot") {
        const c = byRef[refs[0]];
        out.push(group(refs[0], indH(x, x + w, yB, true) + lbl(x + w / 2, yB + 26, c, "middle")));
      } else out.push(line(x, yB, x + w, yB));
      x += w;
      continue;
    }
    if (st.type === "cmc") {
      const c = byRef[st.refs[0]], w = Math.max(138, lw(c) + 20);
      let s = indH(x, x + w, yT) + indH(x, x + w, yB, true);
      const cx = x + w / 2;
      s += `<line x1="${cx - 4}" y1="${yT + 14}" x2="${cx - 4}" y2="${yB - 14}" class="core"/><line x1="${cx + 4}" y1="${yT + 14}" x2="${cx + 4}" y2="${yB - 14}" class="core"/>`;
      s += `<circle cx="${x + 30}" cy="${yT - 12}" r="2.6" class="dotp"/><circle cx="${x + 30}" cy="${yB + 12}" r="2.6" class="dotp"/>`;
      s += lbl(cx, yT - 44, c, "middle");
      out.push(group(st.refs[0], s));
      x += w;
      continue;
    }
    if (st.type === "y") {
      const c1 = byRef[st.refs[0]], c2 = byRef[st.refs[1]];
      const w = Math.max(110, 32 + 22 + Math.max(lw(c1), lw(c2)) + 14), cx = x + 32;
      out.push(rails(x, x + w) + dot(cx, yT) + dot(cx, yB));
      out.push(group(st.refs[0], capV(cx, yT, yM, c1) + lbl(cx + 22, (yT + yM) / 2 - 8, c1)));
      out.push(group(st.refs[1], capV(cx, yM, yB, c2) + lbl(cx + 22, (yM + yB) / 2 - 8, c2)));
      out.push(dot(cx, yM) + line(cx, yM, cx - 22, yM) + line(cx - 22, yM, cx - 22, yM + 10) + earth(cx - 22, yM + 10));
      x += w;
      continue;
    }
    if (st.type === "conv") {
      out.push(rails(x, x + 22));
      x += 22;
      const c = byRef["CIN"];
      const bw = 150;
      out.push(`<rect x="${x}" y="${yT - 38}" width="${bw}" height="${yB - yT + 76}" rx="8" class="box conv"/>`);
      out.push(text(x + bw / 2, yM - 26, mode === "ac" ? "Bridge +" : "Switching", "t boxt", "middle"));
      out.push(text(x + bw / 2, yM - 8, "converter", "t boxt", "middle"));
      if (c) out.push(group("CIN", text(x + bw / 2, yM + 16, "C_in " + compLabel(c), "t small", "middle")));
      out.push(text(x + bw / 2, yM + 32, "(noise source)", "t small", "middle"));
      x += bw + 20;
    }
  }
  const W = x + 10, H = yB + 80;
  el.innerHTML = `<svg viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" style="max-width:max(100%, ${Math.round(W * 0.8)}px);height:auto" xmlns="http://www.w3.org/2000/svg" font-family="Segoe UI, system-ui, sans-serif">
  <style>
    .w{stroke:${SC.stroke};stroke-width:1.6}
    .sym{stroke:${SC.stroke};stroke-width:2}
    .core{stroke:${SC.stroke};stroke-width:1.6}
    .dot{fill:${SC.stroke}} .dotp{fill:${SC.stroke}}
    .box{fill:#f8fafc;stroke:#94a3b8;stroke-width:1.4}
    .box.conv{fill:#eff6ff;stroke:#93c5fd}
    .t{font-size:12px;fill:#334155}
    .t.ref{font-weight:700;fill:#0f172a;font-size:12.5px}
    .t.val{fill:#1d4ed8;font-weight:600}
    .t.mpn{fill:#64748b;font-size:10.5px}
    .t.small{fill:#64748b;font-size:11px}
    .t.boxt{font-weight:600;fill:#0f172a;font-size:12.5px}
    .t.term{font-weight:700;font-size:14px;fill:#0f172a}
    .schem-part{cursor:pointer}
    .schem-part:hover .sym{stroke:${SC.accent}}
    .schem-part:hover .t{fill:${SC.accent}}
  </style>
  ${out.join("\n")}
</svg>`;
  el.querySelectorAll(".schem-part").forEach((g) => g.addEventListener("click", () => onClick && onClick(g.dataset.ref)));

  function earth(ex, ey) {
    return `<line x1="${ex - 11}" y1="${ey}" x2="${ex + 11}" y2="${ey}" class="w"/><line x1="${ex - 7}" y1="${ey + 5}" x2="${ex + 7}" y2="${ey + 5}" class="w"/><line x1="${ex - 3}" y1="${ey + 10}" x2="${ex + 3}" y2="${ey + 10}" class="w"/>`;
  }
}
