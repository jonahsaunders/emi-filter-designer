package engine

import (
	"fmt"
	"math/cmplx"
	"os"
	"testing"
)

// Writes the SPICE netlist of the default DC/AC designs plus the engine's own
// LISN transfer functions, for cross-checking with an independent solver.
func TestExportForXCheck(t *testing.T) {
	dir := os.Getenv("XCHECK_DIR")
	if dir == "" {
		t.Skip("XCHECK_DIR not set")
	}
	for _, mode := range []string{"dc", "ac"} {
		p := DefaultParams(mode)
		out, _ := Design(p)
		os.WriteFile(dir+"/"+mode+".cir", []byte(SpiceNetlist(p, out.Circuit)), 0o644)
		nl := buildNet(p, out.Circuit, netOpts{filter: true, conv: true, lisn: true})
		f, _ := os.Create(dir + "/" + mode + "_engine.txt")
		for _, fr := range []float64{150e3, 800e3, 3e6, 27e6, 90e6} {
			s, _ := nl.Solve(fr, []map[string]complex128{{"dm": 1}, {"cm": 1}})
			fmt.Fprintf(f, "%g %g %g %g %g\n", fr, cmplx.Abs(s[0].V("LM1")), cmplx.Abs(s[0].V("LM2")), cmplx.Abs(s[1].V("LM1")), cmplx.Abs(s[1].V("LM2")))
		}
		f.Close()
	}
}
