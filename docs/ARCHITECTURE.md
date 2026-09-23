# Architecture

This document explains how EMI Filter Designer is put together, both the code and the engineering models behind it.

## Overview

```
                ┌──────────────────────── web/ (HTML/CSS/JS UI) ────────────────────────┐
                │  app.js (state, forms, tables)   chart.js (log plots)   schematic.js    │
                └───────────────┬───────────────────────────────────────┬────────────────┘
                                │ api(path, body)                       │
                 desktop build  │                         web build     │ window.STATIC_API
                                ▼                                       ▼
              main.go: HTTP server on 127.0.0.1      static-api.js → Web Worker → cmd/wasm (Go→WASM)
                                │                                       │
                                └──────────────┬────────────────────────┘
                                               ▼
                               engine/  (design, simulation, export)
                               parts/   (Mouser/DigiKey parsing & ranking)
```

The UI never computes physics itself. It calls a small JSON API. The **desktop build** serves that API over HTTP from `main.go`. The **web build** runs the same Go packages compiled to WebAssembly in a Web Worker, and `web/static-api.js` emulates the same endpoints. Because both builds run identical engine code, they give identical results.

### API

| Endpoint | Body | Returns |
|---|---|---|
| `GET /api/init` | none | default parameters, standards, settings status |
| `POST /api/design` | `{params}` | `{circuit, log, sim}`, a freshly synthesized filter |
| `POST /api/simulate` | `{params, circuit}` | `{sim}` for a (user-edited) circuit |
| `POST /api/parts` | `{comp, keyword, sources[]}` | ranked candidates from library / Mouser / DigiKey |
| `POST /api/applypart` | `{comp, cand}` | the component updated with the chosen part |
| `POST /api/export/spice` | `{params, circuit}` | SPICE netlist text |
| `POST /api/export/bom` | `{params, circuit}` | BOM CSV text |
| `GET/POST /api/settings` | keys | masked settings status |
| `GET /api/testkeys` | none | result of a test search on each distributor |
| `POST /api/ping`, `/api/quit` | none | keep-alive (the desktop app exits when the window closes) |

## Data model (`engine/types.go`)

- **`Params`**: everything the user enters: mode (`dc`/`ac`), standard, detector, margins, topology, power, switching frequency, input range, noise-model inputs (rise time, `Cp`, converter input capacitor) and filter options.
- **`Comp`**: one BOM line: kind (`cap`, `ind`, `res`, `cmc`), value, quantity in parallel, parasitics (ESR, ESL, DCR, SRF, leakage), voltage rating and DC bias, part requirements, the node names it connects to, and the selected `PartInfo`.
- **`Circuit`**: the list of `Comp`s plus a `Layout` (a sequence of ladder stages) that the UI uses to draw the schematic. Filter input nodes are `T0`/`B0` (top/bottom line at the LISN); the output nodes connect to the converter. `CIN` is the converter's own input capacitor (`InBOM=false`).

## Simulation engine

### MNA solver (`engine/mna.go`)
This is a complex-valued modified nodal analysis. It supports:
- R and C elements, stamped as admittances;
- inductors, which get their own branch current so they can be **mutually coupled** (`K`), as common-mode chokes need;
- tagged current and voltage sources.

Several excitations can be solved against a single factorization (multiple right-hand sides). This is used to get the DM and CM responses in one solve. Solving uses Gaussian elimination with partial pivoting, and a tiny g<sub>min</sub> on every node keeps floating nodes from making the matrix singular.

### Component models (`engine/netbuild.go`)
| Component | Model |
|---|---|
| Capacitor | ESR – ESL – C in series. C is the **effective** capacitance after class-II MLCC DC-bias derating, times Qty (ESR/ESL divided by Qty). |
| Inductor | DCR in series with L ‖ C<sub>p</sub> ‖ R<sub>p</sub>. C<sub>p</sub> comes from the SRF; R<sub>p</sub> = Q·ω<sub>SRF</sub>·L sets the peak impedance. |
| CM choke | Two coupled windings with coupling k = 1 − L<sub>leak,DM</sub>/(2L), each with DCR, winding capacitance (from SRF) and a core-loss resistor. |
| Resistor | Ideal, with optional ESL. |

### Test setup
- **Artificial networks:**
  - CISPR 16 **50 µH + 5 Ω** V-network for mains/CISPR 32.
  - CISPR 25 **5 µH** AN with its 1 µF supply-side capacitor, plus a small battery/cable impedance.
  - Both use a 0.1 µF coupling capacitor into the 50 Ω receiver (node `LM1`/`LM2`).
  - With "return grounded at the EUT", only the positive line has a network.
- **Converter:**
  - its input capacitor across the filter output;
  - a DM current source across it;
  - a CM voltage source from the converter return to a switch-node node, coupled to chassis/PE through `Cp`.

### Noise sources (`engine/noise.go`)
The noise spectrum is evaluated at the min / nominal / max input (DC) or min / max line voltage (AC). For each harmonic, the largest value across those operating points is used.

- **Pulsed input current** (buck, buck-boost, flyback): a trapezoid with height I<sub>in</sub>/D, duty D and edge time t<sub>r</sub>:
  c<sub>n</sub> = 2·A·D·|sinc(nπD)|·|sinc(nπt<sub>r</sub>/T)|
- **Continuous input current** (boost, PFC): a triangle with peak-to-peak ripple ΔI:
  c<sub>n</sub> = ΔI·|sin(nπD)| / (π²n²D(1−D))
- **CM source:** the switch-node voltage swing as a trapezoid with the same formula.

### Analyses (`engine/analysis.go`)
- **Emissions:**
  - each harmonic is solved twice, once for DM and once for CM;
  - each receiver voltage is converted to dBµV (rms);
  - per line, the total is the worst-case magnitude sum |V<sub>DM</sub>| + |V<sub>CM</sub>|;
  - the margin is taken against the selected detector's limit.
- **In-circuit attenuation:** receiver voltage without the filter divided by receiver voltage with it, swept continuously.
- **50 Ω / 50 Ω insertion loss:** DM (balanced source and load) and CM (lines tied together), CISPR 17 style.
- **Stability:**
  - |Z<sub>out</sub>| is computed at the converter terminals, including the AN and C<sub>in</sub>;
  - it is compared with |Z<sub>in</sub>| = V<sub>min</sub>²/P<sub>in</sub>, the converter's negative incremental input resistance;
  - the check runs up to about f<sub>sw</sub>/5, a proxy for the control bandwidth.

## Automatic design (`engine/design.go`)

### DC
1. Simulate with no filter and compute the attenuation each harmonic needs.
2. **DM π filter:**
   - The characteristic impedance R<sub>0</sub> is capped by the Middlebrook target, allowing for the damped peak R<sub>0</sub>·√(2(2+n)/n).
   - L is taken from real inductors rated for the current and saturation current; C is sized for the corner frequency f<sub>0</sub>.
   - MLCC banks are sized for **effective** capacitance at the DC bias.
   - f<sub>0</sub> is found by bracketing and bisection: the *highest* corner frequency that still meets the DM target, which gives the smallest parts.
   - The filter automatically moves to 2 stages when the corner frequency gets very low or the MLCC banks become impractical.
3. **CM:**
   - When the CM margin is short, the designer tries CM chokes from smallest to largest, adding line-to-chassis caps if a chassis connection is allowed.
   - It then re-optimizes the DM filter, because the choke's leakage inductance adds DM attenuation.
4. **Damping:**
   - First try Erickson's optimum parallel R–C<sub>d</sub> leg with n = 4.
   - If the Middlebrook margin is still missed, which is usually the AN/supply inductance resonating with the filter caps, search C<sub>d</sub> ∈ {1 … 220 µF} × R<sub>d</sub> for the smallest C<sub>d</sub> that meets the margin.
   - Electrolytic ESR counts toward R<sub>d</sub>.
5. Re-check the total (DM + CM) margin and tighten if needed.

### AC
1. Size the Y capacitors from the leakage-current budget: C<sub>Y</sub> ≤ I<sub>leak</sub> / (2π·f<sub>line</sub>·V<sub>max</sub>), rounded down to a standard value.
2. **CM:** evaluate every single choke and every pair of chokes rated for the line current. Pick the cheapest configuration (by size rank and DCR) that meets the CM target.
3. **DM:** try X capacitor values up to the maximum allowed, plus optional DM inductors, cheapest first. The CM choke's leakage inductance supplies most of the DM inductance.
4. Check the total with every harmonic, then greedily downsize whatever still passes.
5. Add X-capacitor bleeders with τ ≤ 1 s (two resistors in series for voltage rating).

## Part search (`parts/`)
- **Mouser:** Search API v1 keyword search. Values, voltage, current and DCR are parsed from the description text.
- **DigiKey:** Product Information v4 keyword search with OAuth2 client credentials. Values come from the structured `Parameters`.
- **Ranking** (`Rank`), in order of weight: value mismatch (log distance), then insufficient ratings, stock, DCR and price.
- **Applying a part** (`Apply`) copies its value (if within 5× of the design value), ratings, DCR, SRF, ESR and case size (the MLCC mounted ESL is derived from the case size), plus the purchasing information.

The web version makes the HTTP requests from JavaScript (`web/static-api.js`) and passes the raw JSON to the same Go parsers.

## Exports (`engine/export.go`)
- **SPICE:** portable R/L/C/K primitives with parasitics expanded, the LISNs, and `.param dm/cm` AC sources. PULSE-source equivalents are included as comments for transient runs, along with the harmonic amplitudes the designer used.
- **BOM:** CSV with a UTF-8 BOM (so Excel shows Ω and µ correctly), ratings, requirements and distributor links.

## Desktop shell (`main.go`)
- Listens on `127.0.0.1` on a random port. Requests addressed to any other host name are refused, which guards against DNS rebinding.
- Opens the UI with `msedge --app=… --user-data-dir=%APPDATA%\EMI Filter Designer\window`, falling back to Chrome and then the default browser.
- The page pings every 5 s. Closing the window sends `/api/quit` and the process exits a few seconds later; with no ping for 3 minutes it also exits.
- MIME types for `.js`/`.css` are registered explicitly, because Windows can map them wrongly in the registry.

## Web build (`cmd/wasm`, `tools/build_pages.py`)
- `GOOS=js GOARCH=wasm` builds the engine, exposing a single `emifCall(fn, jsonArg)` function.
- The build script gzips and base64-encodes the `.wasm` file and inlines it, together with Go's `wasm_exec.js`, the CSS and all scripts, into **one** `index.html`.
- At start-up the page decompresses the engine (`DecompressionStream`) and runs it in a Web Worker, so the UI stays responsive during a design run.
