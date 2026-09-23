//go:build js && wasm

// WebAssembly build of the EMI Filter Designer engine for the static
// (GitHub Pages) version.  It runs inside a Web Worker and exposes one
// function, emifCall(fn, jsonArg) -> jsonResult.
package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"syscall/js"

	"emifilter/engine"
	"emifilter/parts"
)

type req struct {
	Params   engine.Params     `json:"params"`
	Circuit  *engine.Circuit   `json:"circuit"`
	Comp     engine.Comp       `json:"comp"`
	Cand     parts.Candidate   `json:"cand"`
	Raw      string            `json:"raw"`
	Currency string            `json:"currency"`
	Results  []parts.Candidate `json:"results"`
}

func call(fn string, arg string) (out any, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("internal error: %v", e)
		}
	}()
	var r req
	if arg != "" {
		if err := json.Unmarshal([]byte(arg), &r); err != nil {
			return nil, err
		}
	}
	switch fn {
	case "init":
		return map[string]any{
			"defaults":  map[string]engine.Params{"dc": engine.DefaultParams("dc"), "ac": engine.DefaultParams("ac")},
			"standards": engine.Standards,
			"library":   len(engine.Library),
		}, nil
	case "design":
		o, err := engine.Design(r.Params)
		if err != nil {
			return nil, err
		}
		sim, err := engine.Simulate(r.Params, o.Circuit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"circuit": o.Circuit, "log": o.Log, "sim": sim}, nil
	case "simulate":
		if r.Circuit == nil {
			return nil, fmt.Errorf("no circuit")
		}
		sim, err := engine.Simulate(r.Params, r.Circuit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"sim": sim}, nil
	case "keywords":
		return map[string]any{"keyword": engine.SearchKeywords(r.Comp)}, nil
	case "library":
		return map[string]any{"results": parts.FromLibrary(engine.Candidates(r.Comp))}, nil
	case "parseMouser":
		c, err := parts.ParseMouser([]byte(r.Raw))
		if err != nil {
			return nil, err
		}
		return map[string]any{"results": c}, nil
	case "parseDigikey":
		c, err := parts.ParseDigikey([]byte(r.Raw), r.Currency)
		if err != nil {
			return nil, err
		}
		return map[string]any{"results": c}, nil
	case "rank":
		return map[string]any{"results": parts.Rank(r.Comp, r.Results)}, nil
	case "applypart":
		return map[string]any{"comp": parts.Apply(r.Comp, r.Cand)}, nil
	case "spice":
		return map[string]any{"text": engine.SpiceNetlist(r.Params, r.Circuit)}, nil
	case "bom":
		return map[string]any{"text": "\xef\xbb\xbf" + engine.BOMCSV(r.Params, r.Circuit)}, nil
	}
	return nil, fmt.Errorf("unknown function %q", fn)
}

func main() {
	js.Global().Set("emifCall", js.FuncOf(func(this js.Value, a []js.Value) any {
		fn, arg := a[0].String(), ""
		if len(a) > 1 {
			arg = a[1].String()
		}
		out, err := call(fn, arg)
		if err != nil {
			b, _ := json.Marshal(map[string]string{"error": err.Error()})
			return string(b)
		}
		b, err := json.Marshal(out)
		if err != nil {
			return `{"error":` + fmt.Sprintf("%q", strings.TrimSpace(err.Error())) + `}`
		}
		return string(b)
	}))
	js.Global().Call("postMessage", map[string]any{"ready": true})
	select {}
}
