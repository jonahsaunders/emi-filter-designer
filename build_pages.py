#!/usr/bin/env python3
"""Assemble the single-file GitHub Pages build (index.html) from web/ and the
WebAssembly engine.  Usage: python3 tools/build_pages.py <engine.wasm> <out.html>"""
import base64, gzip, pathlib, subprocess, sys

root = pathlib.Path(__file__).resolve().parent.parent
wasm_path, out = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
goroot = subprocess.check_output(["go", "env", "GOROOT"], text=True).strip()
exec_js = pathlib.Path(goroot, "lib", "wasm", "wasm_exec.js")
if not exec_js.exists():
    exec_js = pathlib.Path(goroot, "misc", "wasm", "wasm_exec.js")

web = root / "web"
html = (web / "index.html").read_text(encoding="utf-8")
read = lambda n: (web / n).read_text(encoding="utf-8")
for name, text in [("css", read("style.css")), ("exec", exec_js.read_text(encoding="utf-8"))] + \
        [(n, read(n)) for n in ("static-api.js", "chart.js", "schematic.js", "app.js")]:
    bad = "</style" if name == "css" else "</script"
    assert bad not in text.lower(), name

b64 = base64.b64encode(gzip.compress(wasm_path.read_bytes(), 9)).decode()
b64 = "\n".join(b64[i:i + 16384] for i in range(0, len(b64), 16384))

html = html.replace('<link rel="stylesheet" href="style.css">', "<style>\n" + read("style.css") + "\n</style>")
html = html.replace('<meta name="viewport"', '<meta name="description" content="Design, simulate and source front-end EMI filters for DC/DC and AC mains power supplies (CISPR 25 / CISPR 32).">\n<meta name="viewport"')
scripts = (
    '<script type="text/plain" id="wasm-exec">\n' + exec_js.read_text(encoding="utf-8") + "\n</script>\n"
    '<script type="application/octet-stream" id="wasm-b64">\n' + b64 + "\n</script>\n"
    + "".join(f"<script>\n{read(n)}\n</script>\n" for n in ("static-api.js", "chart.js", "schematic.js", "app.js"))
)
old = '<script src="chart.js"></script>\n<script src="schematic.js"></script>\n<script src="app.js"></script>'
assert old in html
html = html.replace(old, scripts)
out.parent.mkdir(parents=True, exist_ok=True)
out.write_text(html, encoding="utf-8")
print(f"wrote {out} ({out.stat().st_size/1e6:.2f} MB)")
