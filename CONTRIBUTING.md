# Contributing to EMI Filter Designer

Thanks for your interest! Bug reports, part-library additions, new standards, model improvements and UI polish are all welcome.

## Ground rules

- Be kind and constructive. See the [Code of Conduct](CODE_OF_CONDUCT.md).
- For anything bigger than a small fix, **open an issue first** so we can agree on the approach.
- Keep the project dependency-free. The engine uses only the Go standard library, and the UI is plain HTML/CSS/JS with no build step. Please don't add frameworks or third-party modules without discussing it first.
- Engineering claims need a source. If you add or change part data, limit values or model equations, cite the datasheet, standard or textbook in the code comment or the pull request.

## Development setup

You need:
- [Go](https://go.dev/dl/) 1.22 or later
- Python 3, only to build the single-file web version
- Any Chromium-based browser (Edge or Chrome), recommended for the app window

```sh
git clone https://github.com/<you>/emi-filter-designer.git
cd emi-filter-designer
go test ./...                 # run the test suite
go run . -dev                 # start the app; UI files are served live from ./web
```

In `-dev` mode:
- edits to `web/*.js`, `web/*.css` and `web/index.html` show up when you reload the page,
- the server keeps running after you close the window.

Changes to Go code need a restart. Useful flags:

| Flag | Meaning |
|---|---|
| `-dev` | Serve `./web` from disk and never auto-exit |
| `-nobrowser` | Don't open a window; open the printed URL yourself |
| `-port 8080` | Use a fixed port instead of a random one |

To try the **web (WebAssembly) version**, run `./build-pages.sh` and open `pages/index.html`.

## Where things live

| You want to… | Look at |
|---|---|
| Add a part to the built-in library | `engine/library.go` → `Library` |
| Add or fix a standard's limit lines | `engine/limits.go` |
| Change the noise-source model | `engine/noise.go` |
| Change how components are modelled (parasitics) | `engine/netbuild.go` → `expandComp` |
| Change the automatic design algorithm | `engine/design.go` (`designDC`, `designAC`) |
| Add a result / chart | `engine/analysis.go` + `web/app.js` |
| Change the distributor search | `parts/parts.go` (desktop) and `web/static-api.js` (web version) |
| Change the UI | `web/` |

[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) explains how the pieces fit together.

## Common contributions

### Adding parts to the library

Each entry is a `LibPart` in `engine/library.go`. Helper constructors exist for common families: `mlcc(...)`, `xal(...)` and `wecmb(...)`. Please:

1. Use the **exact manufacturer part number**. Check it on Mouser or DigiKey.
2. Take values from the **current datasheet**. For inductors, give typical DCR, Isat, Irms and SRF. For capacitors, give the voltage rating, dielectric class, case size and, if known, ESR/ESL.
3. Put the datasheet link or document number in the pull request description.
4. Run `go test ./...`. The design scenarios may pick your new part, which is fine; just check the results still look sensible.

### Adding a standard

Add a `Standard` to `Standards` in `engine/limits.go` with its segments (`LimitSeg`) and the LISN type (`"50uH"` or `"5uH"`). Sloped segments interpolate linearly in log-frequency. Add a check to `TestLimits` for at least one frequency per band.

### Changing the models or the design algorithm

- Add or update tests in `engine/engine_test.go`. `TestScenarios` runs eight end-to-end designs and logs their margins. Compare the output before and after your change (`go test ./engine -run Scenarios -v`) and summarize the differences in the pull request.
- If you touch `netbuild.go` or `export.go`, the SPICE export must stay consistent with the internal solver. `engine/xcheck_test.go` writes both the netlist and the engine's transfer functions so they can be compared with an external simulator:
  ```sh
  XCHECK_DIR=/tmp/x go test ./engine -run XCheck
  ```
  Then run `/tmp/x/dc.cir` and `/tmp/x/ac.cir` in LTspice or ngspice.

## Code style

- Go: run `gofmt` (CI enforces it) and `go vet ./...`. Keep functions small enough to read, and comment the *why* (physics, standard clause) more than the *what*.
- JavaScript: plain ES2020 with no transpiler. Use 2-space indentation, double quotes and `"use strict"`. Don't use `innerHTML` with unescaped user or API data; use `esc()`.
- Keep the desktop app and the web version working. The web version calls the same engine through `cmd/wasm/main.go`, so if you add an HTTP endpoint in `main.go`, add the matching case in `web/static-api.js` and `cmd/wasm/main.go`.

## Pull request checklist

- [ ] `go test ./...` passes
- [ ] `gofmt -l .` prints nothing
- [ ] The desktop app works: `go run .`
- [ ] The web version still builds: `./build-pages.sh`, and `pages/index.html` works
- [ ] Sources are cited for any new part data, limits or equations
- [ ] Screenshots are included for UI changes

## Releasing (maintainers)

1. Update `CHANGELOG.md` and the `version` constant in `main.go` (and in `web/static-api.js`).
2. Tag and push: `git tag v1.0.1 && git push origin v1.0.1`.
3. The **Release** workflow builds `EMIFilterDesigner.exe`, the Windows zip and `index.html`, and attaches them to a GitHub Release.

### Windows icon and version info

`rsrc_windows_amd64.syso` is compiled from `winres/app.rc`. If you change the icon or version info, regenerate it with LLVM's resource compiler:

```sh
cd winres && llvm-windres --target=pe-x86-64 app.rc -O coff -o ../rsrc_windows_amd64.syso
```

## Reporting bugs

Please use the issue templates. Include:
- your inputs (use **Export → Save project** and attach the `.emif.json` file),
- what you expected,
- what happened,
- a measurement if the prediction disagrees with a real scan.

Scan data is hugely valuable for improving the models.
