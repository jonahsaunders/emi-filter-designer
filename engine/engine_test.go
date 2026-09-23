package engine

import (
	"math"
	"math/cmplx"
	"testing"
	"time"
)

// RC low-pass: |H| = 1/sqrt(1+(wRC)^2)
func TestMNA_RC(t *testing.T) {
	nl := NewNetlist()
	nl.V("V1", "in", "0", "s")
	nl.R("R1", "in", "out", 1000)
	nl.C("C1", "out", "0", 1e-6)
	f := 1000.0
	s, err := nl.Solve(f, []map[string]complex128{{"s": 1}})
	if err != nil {
		t.Fatal(err)
	}
	got := cmplx.Abs(s[0].V("out"))
	w := 2 * math.Pi * f
	want := 1 / math.Sqrt(1+math.Pow(w*1000*1e-6, 2))
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("RC: got %g want %g", got, want)
	}
}

// Series LC driven by current source: resonance at 1/(2pi sqrt(LC)).
func TestMNA_LC_Resonance(t *testing.T) {
	nl := NewNetlist()
	nl.I("I1", "0", "a", "s")
	nl.R("R", "a", "0", 1e6)
	nl.L("L1", "a", "b", 10e-6)
	nl.C("C1", "b", "0", 1e-6)
	f0 := 1 / (2 * math.Pi * math.Sqrt(10e-6*1e-6))
	s, _ := nl.Solve(f0, []map[string]complex128{{"s": 1}})
	if v := cmplx.Abs(s[0].V("a")); v > 1e-3 {
		t.Fatalf("series LC at resonance should short: |V| = %g", v)
	}
}

// Coupled inductors: CM choke with k~1: DM inductance = 2L(1-k).
func TestMNA_Coupled(t *testing.T) {
	L := 1e-3
	k := 0.99
	nl := NewNetlist()
	nl.V("V1", "a1", "b1", "s")
	nl.L("La", "a1", "a2", L)
	nl.L("Lb", "b1", "b2", L) // DM: current returns backwards through the second winding
	nl.K("K", "La", "Lb", k)
	nl.R("Rl", "a2", "b2", 1e-6)
	nl.R("Rg", "b1", "0", 1)
	f := 1e3
	s, _ := nl.Solve(f, []map[string]complex128{{"s": 1}})
	// current = (V(a2)-V(b2))/1e-6
	i := (s[0].V("a2") - s[0].V("b2")) / 1e-6
	z := cmplx.Abs(1 / i)
	want := 2 * math.Pi * f * 2 * L * (1 - k)
	if math.Abs(z-want)/want > 0.01 {
		t.Fatalf("DM leakage impedance got %g want %g", z, want)
	}
}

func TestLimits(t *testing.T) {
	s := GetStandard("cispr32_b")
	_, qp, av, ok := s.Limit(150e3)
	if !ok || math.Abs(qp-66) > 0.01 || math.Abs(av-56) > 0.01 {
		t.Fatalf("CISPR32B at 150k: %g %g", qp, av)
	}
	_, qp, av, _ = s.Limit(500e3)
	if math.Abs(qp-56) > 0.01 || math.Abs(av-46) > 0.01 {
		t.Fatalf("CISPR32B at 500k: %g %g", qp, av)
	}
	c5 := GetStandard("cispr25_c5")
	if _, _, av, ok := c5.Limit(1e6); !ok || av != 34 {
		t.Fatalf("CISPR25 C5 MW AV %g", av)
	}
	if _, _, _, ok := c5.Limit(400e3); ok {
		t.Fatalf("400 kHz should be outside CISPR 25 bands")
	}
}

func TestHarmonics(t *testing.T) {
	// 50% square wave, tr->0: c1 = 2A/pi
	a := trapHarm(1, 0.5, 1e-15, 1e-6, 1)
	if math.Abs(a-2/math.Pi) > 1e-6 {
		t.Fatalf("trap c1 %g", a)
	}
	b := triHarm(1, 0.5, 1)
	if math.Abs(b-4/(math.Pi*math.Pi)) > 1e-9 {
		t.Fatalf("tri c1 %g", b)
	}
}

func TestDesignDC(t *testing.T) {
	p := DefaultParams("dc")
	t0 := time.Now()
	out, err := Design(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("design time %v", time.Since(t0))
	for _, l := range out.Log {
		t.Log(l)
	}
	for _, c := range out.Circuit.Comps {
		mpn := ""
		if c.Part != nil {
			mpn = c.Part.MPN
		}
		t.Logf("%-5s %-4s qty%d %-10s %-22s %v %s", c.Ref, c.Kind, c.Qty, FmtEng(c.Value, ""), mpn, c.Nodes, c.Role)
	}
	t0 = time.Now()
	r, err := Simulate(p, out.Circuit)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sim time %v margin %.1f dm %.1f cm %.1f unf %.1f stab %.1f dB drop %.3fV loss %.2fW", time.Since(t0), r.Margin, r.MarginDM, r.MarginCM, r.UnfMargin, r.Stab.MarginDB, r.DropV, r.LossW)
	for _, w := range r.Warnings {
		t.Log("WARN:", w)
	}
}

func TestDesignAC(t *testing.T) {
	p := DefaultParams("ac")
	t0 := time.Now()
	out, err := Design(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("design time %v", time.Since(t0))
	for _, l := range out.Log {
		t.Log(l)
	}
	for _, c := range out.Circuit.Comps {
		mpn := ""
		if c.Part != nil {
			mpn = c.Part.MPN
		}
		t.Logf("%-5s %-4s qty%d %-10s %-22s %v %s", c.Ref, c.Kind, c.Qty, FmtEng(c.Value, ""), mpn, c.Nodes, c.Role)
	}
	r, err := Simulate(p, out.Circuit)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("margin %.1f dm %.1f cm %.1f unf %.1f stab %.1f dB drop %.3fV loss %.2fW leak %.2fmA", r.Margin, r.MarginDM, r.MarginCM, r.UnfMargin, r.Stab.MarginDB, r.DropV, r.LossW, r.CyLeakageMA)
	for _, w := range r.Warnings {
		t.Log("WARN:", w)
	}
}

func TestScenarios(t *testing.T) {
	type sc struct {
		name string
		mod  func(p *Params)
		mode string
	}
	cases := []sc{
		{"dc-buck-48V-200W-cispr32a", func(p *Params) {
			p.Standard = "cispr32_a"
			p.VinMin, p.VinNom, p.VinMax, p.VinAbsMax = 36, 48, 60, 75
			p.Pout = 200
			p.Vout = 12
			p.Fsw = 250e3
		}, "dc"},
		{"dc-boost-12V-60W-c3", func(p *Params) {
			p.Standard = "cispr25_c3"
			p.Topology = "boost"
			p.Vout = 24
			p.Pout = 60
			p.Fsw = 2.1e6
		}, "dc"},
		{"dc-buck-grounded-c5", func(p *Params) { p.ReturnGrounded = true; p.Fsw = 2.2e6; p.Pout = 10 }, "dc"},
		{"dc-flyback-24V-cispr32b", func(p *Params) {
			p.Standard = "cispr32_b"
			p.Topology = "flyback"
			p.VinMin, p.VinNom, p.VinMax, p.VinAbsMax = 18, 24, 32, 40
			p.Pout = 15
			p.Fsw = 150e3
		}, "dc"},
		{"dc-buck-2stage", func(p *Params) { p.Stages = "2" }, "dc"},
		{"ac-pfc-300W", func(p *Params) {
			p.Topology = "pfc"
			p.Pout = 300
			p.Fsw = 65e3
			p.PF = 0.98
			p.CinC = 1e-6
			p.CinESR = 0.03
			p.CinESL = 15e-9
			p.Cp = 20e-12
		}, "ac"},
		{"ac-flyback-20W-classA", func(p *Params) { p.Standard = "cispr32_a"; p.Pout = 20; p.Fsw = 65e3 }, "ac"},
		{"ac-flyback-lowleak", func(p *Params) { p.LeakageLimit = 0.1e-3 }, "ac"},
	}
	for _, c := range cases {
		p := DefaultParams(c.mode)
		c.mod(&p)
		t0 := time.Now()
		out, err := Design(p)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		r, err := Simulate(p, out.Circuit)
		if err != nil {
			t.Fatalf("%s sim: %v", c.name, err)
		}
		for _, l := range out.Log {
			t.Log("   ", l)
		}
		t.Logf("%-28s %6v margin %5.1f dm %5.1f cm %5.1f stab %5.1f | %s", c.name, time.Since(t0).Round(time.Millisecond), r.Margin, r.MarginDM, r.MarginCM, r.Stab.MarginDB, out.Circuit.Description)
		for _, cp := range out.Circuit.Comps {
			if cp.InBOM {
				mpn := "(generic)"
				if cp.Part != nil && cp.Part.MPN != "" {
					mpn = cp.Part.MPN
				}
				t.Logf("      %-5s %dx %-8s %s", cp.Ref, cp.Qty, FmtEng(cp.Value, ""), mpn)
			}
		}
	}
}
