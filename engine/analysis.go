package engine

import (
	"fmt"
	"math"
	"math/cmplx"
)

type HarmResult struct {
	F        []JF `json:"f"`
	DM       []JF `json:"dm"`
	CM       []JF `json:"cm"`
	Total    []JF `json:"total"`
	UnfDM    []JF `json:"unfDm"`
	UnfCM    []JF `json:"unfCm"`
	UnfTotal []JF `json:"unfTotal"`
	Limit    []JF `json:"limit"`
}

type SweepResult struct {
	F  []JF `json:"f"`
	DM []JF `json:"dm"`
	CM []JF `json:"cm"`
}

type StabResult struct {
	F        []JF    `json:"f"`
	Zout     []JF    `json:"zout"`    // dB-ohm
	ZoutUnd  []JF    `json:"zoutUnd"` // without damping network
	ZinDB    float64 `json:"zinDb"`
	Zin      float64 `json:"zin"`
	PeakDB   float64 `json:"peakDb"`
	PeakF    float64 `json:"peakF"`
	MarginDB float64 `json:"marginDb"`
	Fcheck   float64 `json:"fcheck"`
	Pass     bool    `json:"pass"`
}

type SimResult struct {
	Standard    Standard    `json:"standard"`
	Detector    string      `json:"detector"`
	Harm        HarmResult  `json:"harm"`
	Margin      float64     `json:"margin"` // worst (limit - total) in dB, over harmonics with a limit
	MarginF     float64     `json:"marginF"`
	MarginDM    float64     `json:"marginDm"`
	MarginCM    float64     `json:"marginCm"`
	UnfMargin   float64     `json:"unfMargin"`
	Required    float64     `json:"required"`
	Pass        bool        `json:"pass"`
	Atten       SweepResult `json:"atten"`
	IL          SweepResult `json:"il"`
	Stab        StabResult  `json:"stab"`
	Irms        float64     `json:"irms"`
	DropV       float64     `json:"dropV"`
	LossW       float64     `json:"lossW"`
	Ops         []OpPoint   `json:"ops"`
	Warnings    []string    `json:"warnings"`
	SingleLISN  bool        `json:"singleLisn"`
	CyLeakageMA float64     `json:"cyLeakageMa"`
}

type harmSet struct {
	f, idm, vcm, tgt []float64
}

func harmonics(p Params, onlyLimited bool) harmSet {
	std := GetStandard(p.Standard)
	fmin := std.Fmin
	if !onlyLimited {
		fmin = math.Min(fmin, p.Fsw)
	}
	f, idm, vcm := NoiseSpectrum(p, fmin*0.999, std.Fmax)
	var hs harmSet
	for i := range f {
		t, ok := std.Target(f[i], p.Detector)
		if onlyLimited && !ok {
			continue
		}
		if !ok {
			t = nan
		}
		hs.f = append(hs.f, f[i])
		hs.idm = append(hs.idm, idm[i])
		hs.vcm = append(hs.vcm, vcm[i])
		hs.tgt = append(hs.tgt, t)
	}
	return hs
}

// lisnLevels returns DM-only, CM-only and worst-case total LISN levels in
// dBuV (worst line) for each harmonic.
func lisnLevels(p Params, c *Circuit, filter bool, hs harmSet) (dm, cm, tot []float64, err error) {
	nl := buildNet(p, c, netOpts{filter: filter, conv: true, lisn: true})
	single := singleLISN(p)
	n := len(hs.f)
	dm, cm, tot = make([]float64, n), make([]float64, n), make([]float64, n)
	for i, fn := range hs.f {
		sols, e := nl.Solve(fn, []map[string]complex128{{"dm": complex(hs.idm[i], 0)}, {"cm": complex(hs.vcm[i], 0)}})
		if e != nil {
			return nil, nil, nil, e
		}
		d1, c1 := cmplx.Abs(sols[0].V("LM1")), cmplx.Abs(sols[1].V("LM1"))
		d, cc, t := d1, c1, d1+c1
		if !single {
			d2, c2 := cmplx.Abs(sols[0].V("LM2")), cmplx.Abs(sols[1].V("LM2"))
			d, cc, t = math.Max(d1, d2), math.Max(c1, c2), math.Max(d1+c1, d2+c2)
		}
		dm[i], cm[i], tot[i] = dbuv(d), dbuv(cc), dbuv(t)
	}
	return
}

// margins returns worst margins (limit - level) and the frequency of the
// worst total margin.
func margins(hs harmSet, dm, cm, tot []float64) (mt, md, mc, fAt float64) {
	mt, md, mc = math.Inf(1), math.Inf(1), math.Inf(1)
	for i := range hs.f {
		if math.IsNaN(hs.tgt[i]) {
			continue
		}
		if v := hs.tgt[i] - tot[i]; v < mt {
			mt, fAt = v, hs.f[i]
		}
		md = math.Min(md, hs.tgt[i]-dm[i])
		mc = math.Min(mc, hs.tgt[i]-cm[i])
	}
	return
}

// quickEval is used inside the design loops.
func quickEval(p Params, c *Circuit, hs harmSet) (mt, md, mc float64) {
	dm, cm, tot, err := lisnLevels(p, c, true, hs)
	if err != nil {
		return -999, -999, -999
	}
	mt, md, mc, _ = margins(hs, dm, cm, tot)
	return
}

func logspace(f1, f2 float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = f1 * math.Pow(f2/f1, float64(i)/float64(n-1))
	}
	return out
}

// zout computes |Zout| (dB-ohm) seen by the converter over f.
func zout(p Params, c *Circuit, f []float64) []float64 {
	nl := buildNet(p, c, netOpts{filter: true, conv: true, lisn: true, test: true})
	out := make([]float64, len(f))
	for i, fn := range f {
		s, err := nl.Solve(fn, []map[string]complex128{{"test": 1}})
		if err != nil {
			out[i] = nan
			continue
		}
		z := cmplx.Abs(s[0].V(c.OutTop) - s[0].V(c.OutBot))
		out[i] = 20 * math.Log10(math.Max(z, 1e-9))
	}
	return out
}

func stabilityFcheck(p Params) float64 {
	// the converter behaves as a negative resistance up to roughly its
	// control bandwidth; check Zout up to fsw/5 (and at least 20 kHz)
	return math.Max(p.Fsw/5, 20e3)
}

func stability(p Params, c *Circuit) StabResult {
	fs := logspace(100, math.Min(10e6, p.Fsw*5), 260)
	z := zout(p, c, fs)
	zin := ConverterZin(p)
	r := StabResult{F: jfs(fs), Zout: jfs(z), Zin: zin, ZinDB: 20 * math.Log10(zin), PeakDB: -999, Fcheck: stabilityFcheck(p)}
	for i, f := range fs {
		if f > r.Fcheck {
			break
		}
		if z[i] > r.PeakDB {
			r.PeakDB, r.PeakF = z[i], f
		}
	}
	// Zout without the damping network, for reference
	nd := *c
	nd.Comps = nil
	hasDamp := false
	for _, cp := range c.Comps {
		if isDamping(cp) {
			hasDamp = true
			continue
		}
		nd.Comps = append(nd.Comps, cp)
	}
	if hasDamp {
		r.ZoutUnd = jfs(zout(p, &nd, fs))
	}
	r.MarginDB = r.ZinDB - r.PeakDB
	r.Pass = r.MarginDB >= p.StabMarginDB
	return r
}

func isDamping(c Comp) bool { return len(c.Ref) >= 2 && (c.Ref[:2] == "RD" || c.Ref[:2] == "CD") }

func sweeps(p Params, c *Circuit) (att, il SweepResult) {
	std := GetStandard(p.Standard)
	fs := logspace(math.Min(10e3, p.Fsw/4), std.Fmax, 300)
	att.F, il.F = jfs(fs), jfs(fs)
	nlF := buildNet(p, c, netOpts{filter: true, conv: true, lisn: true})
	nlU := buildNet(p, c, netOpts{filter: false, conv: true, lisn: true})
	dmIL1 := buildIL(c, "dm", true)
	dmIL0 := buildIL(c, "dm", false)
	cmIL1 := buildIL(c, "cm", true)
	cmIL0 := buildIL(c, "cm", false)
	exc := []map[string]complex128{{"dm": 1}, {"cm": 1}}
	single := singleLISN(p)
	lv := func(s Solution) float64 {
		v := cmplx.Abs(s.V("LM1"))
		if !single {
			v = math.Max(v, cmplx.Abs(s.V("LM2")))
		}
		return math.Max(v, 1e-30)
	}
	for _, f := range fs {
		sf, e1 := nlF.Solve(f, exc)
		su, e2 := nlU.Solve(f, exc)
		if e1 != nil || e2 != nil {
			att.DM = append(att.DM, JF(nan))
			att.CM = append(att.CM, JF(nan))
		} else {
			att.DM = append(att.DM, JF(20*math.Log10(lv(su[0])/lv(sf[0]))))
			att.CM = append(att.CM, JF(20*math.Log10(lv(su[1])/lv(sf[1]))))
		}
		// 50/50 ohm insertion loss
		d1, _ := dmIL1.Solve(f, []map[string]complex128{{"p": 0.5, "n": -0.5}})
		d0, _ := dmIL0.Solve(f, []map[string]complex128{{"p": 0.5, "n": -0.5}})
		c1, _ := cmIL1.Solve(f, []map[string]complex128{{"p": 1}})
		c0, _ := cmIL0.Solve(f, []map[string]complex128{{"p": 1}})
		if d1 != nil && d0 != nil {
			v1 := cmplx.Abs(d1[0].V(c.OutTop) - d1[0].V(c.OutBot))
			v0 := cmplx.Abs(d0[0].V(c.OutTop) - d0[0].V(c.OutBot))
			il.DM = append(il.DM, JF(20*math.Log10(v0/math.Max(v1, 1e-30))))
		} else {
			il.DM = append(il.DM, JF(nan))
		}
		if c1 != nil && c0 != nil {
			v1 := cmplx.Abs(c1[0].V(c.OutTop) + c1[0].V(c.OutBot))
			v0 := cmplx.Abs(c0[0].V(c.OutTop) + c0[0].V(c.OutBot))
			il.CM = append(il.CM, JF(20*math.Log10(v0/math.Max(v1, 1e-30))))
		} else {
			il.CM = append(il.CM, JF(nan))
		}
	}
	return
}

// SeriesResistance returns the DC resistance in the current loop.
func SeriesResistance(c *Circuit) float64 {
	r := 0.0
	for _, cp := range c.Comps {
		if !cp.InBOM {
			continue
		}
		switch cp.Kind {
		case "ind":
			r += cp.DCR
		case "cmc":
			r += 2 * cp.DCR
		}
	}
	return r
}

// Simulate runs the full analysis for display.
func Simulate(p Params, c *Circuit) (*SimResult, error) {
	p = Sanitize(p)
	std := GetStandard(p.Standard)
	hs := harmonics(p, false)
	if len(hs.f) == 0 {
		return nil, fmt.Errorf("no switching harmonics inside %s range - check fsw", std.Name)
	}
	dm, cm, tot, err := lisnLevels(p, c, true, hs)
	if err != nil {
		return nil, err
	}
	udm, ucm, utot, err := lisnLevels(p, c, false, hs)
	if err != nil {
		return nil, err
	}
	r := &SimResult{Standard: std, Detector: p.Detector, Ops: OpPoints(p), SingleLISN: singleLISN(p), Required: p.MarginDB}
	r.Harm = HarmResult{F: jfs(hs.f), DM: jfs(dm), CM: jfs(cm), Total: jfs(tot), UnfDM: jfs(udm), UnfCM: jfs(ucm), UnfTotal: jfs(utot), Limit: jfs(hs.tgt)}
	r.Margin, r.MarginDM, r.MarginCM, r.MarginF = margins(hs, dm, cm, tot)
	r.UnfMargin, _, _, _ = margins(hs, udm, ucm, utot)
	r.Pass = r.Margin >= p.MarginDB
	r.Atten, r.IL = sweeps(p, c)
	r.Stab = stability(p, c)
	r.Irms = InputCurrentMax(p)
	rs := SeriesResistance(c)
	r.DropV = r.Irms * rs
	r.LossW = r.Irms * r.Irms * rs

	// warnings / checks
	for _, cp := range c.Comps {
		if !cp.InBOM {
			continue
		}
		switch cp.Kind {
		case "ind", "cmc":
			if cp.ReqI > 0 && cp.Part != nil && cp.Part.Source == "library" {
				if lp, ok := libByMPN(cp.Part.MPN); ok && lp.IRms < r.Irms {
					r.Warnings = append(r.Warnings, fmt.Sprintf("%s: rated current %.2g A is below the input current %.2g A", cp.Ref, lp.IRms, r.Irms))
				}
			}
		case "cap":
			if cp.ReqV > 0 && cp.VRated > 0 && cp.VRated < cp.ReqV && !cp.ReqVAC {
				r.Warnings = append(r.Warnings, fmt.Sprintf("%s: voltage rating %gV is below the recommended %gV", cp.Ref, cp.VRated, cp.ReqV))
			}
		}
	}
	if p.Mode == "ac" {
		cyTot := 0.0
		for _, cp := range c.Comps {
			if cp.InBOM && cp.Sub == "cer_y" && len(cp.Nodes) > 1 && cp.Nodes[1] == "GND" {
				cyTot = math.Max(cyTot, cp.Value*float64(max(cp.Qty, 1)))
			}
		}
		r.CyLeakageMA = 2 * math.Pi * p.LineFreq * p.VacMax * cyTot * 1e3
		if p.LeakageLimit > 0 && r.CyLeakageMA > p.LeakageLimit*1e3 {
			r.Warnings = append(r.Warnings, fmt.Sprintf("Y-capacitor leakage %.2f mA exceeds the %.2f mA budget", r.CyLeakageMA, p.LeakageLimit*1e3))
		}
	}
	if !r.Stab.Pass {
		r.Warnings = append(r.Warnings, fmt.Sprintf("Middlebrook criterion: filter output impedance peak (%.1f dBΩ at %s) is only %.1f dB below |Zin| = %.2f Ω (target %.0f dB). Increase damping or lower the filter impedance.",
			r.Stab.PeakDB, FmtEng(r.Stab.PeakF, "Hz"), r.Stab.MarginDB, r.Stab.Zin, p.StabMarginDB))
	}
	if p.Fsw < 9e3*2 {
		r.Warnings = append(r.Warnings, "fsw is close to the 9 kHz receiver bandwidth: several harmonics fall into one RBW, measured levels will be higher than predicted.")
	}
	if std.LISN == "5uH" && std.Fmax > 30e6 {
		r.Warnings = append(r.Warnings, "Above ~30 MHz results depend strongly on layout, cable and ground-plane parasitics not modelled here; treat VHF/FM-band predictions as indicative.")
	}
	return r, nil
}

// Sanitize fills missing / invalid values with defaults.
func Sanitize(p Params) Params {
	d := DefaultParams(p.Mode)
	if p.Mode != "ac" {
		p.Mode = "dc"
	}
	fix := func(v *float64, def float64) {
		if *v <= 0 || math.IsNaN(*v) || math.IsInf(*v, 0) {
			*v = def
		}
	}
	fix(&p.Fsw, d.Fsw)
	fix(&p.RiseTime, d.RiseTime)
	fix(&p.Cp, d.Cp)
	fix(&p.Eff, d.Eff)
	if p.Eff > 1 {
		p.Eff /= 100
	}
	fix(&p.Pout, d.Pout)
	fix(&p.Vout, d.Vout)
	fix(&p.Vrefl, d.Vrefl)
	fix(&p.RippleRatio, d.RippleRatio)
	fix(&p.VinMin, d.VinMin)
	fix(&p.VinNom, math.Max(p.VinMin, d.VinNom))
	fix(&p.VinMax, math.Max(p.VinNom, d.VinMax))
	fix(&p.VinAbsMax, p.VinMax)
	if p.VinAbsMax < p.VinMax {
		p.VinAbsMax = p.VinMax
	}
	fix(&p.VacMin, d.VacMin)
	fix(&p.VacMax, math.Max(p.VacMin, d.VacMax))
	fix(&p.LineFreq, 50)
	fix(&p.PF, d.PF)
	fix(&p.MaxCy, d.MaxCy)
	fix(&p.MaxCx, d.MaxCx)
	fix(&p.LeakRatio, d.LeakRatio)
	fix(&p.MaxCyDC, d.MaxCyDC)
	fix(&p.CinC, d.CinC)
	fix(&p.CinESR, d.CinESR)
	if p.CinESL < 0 {
		p.CinESL = 0
	}
	if p.MarginDB < 0 {
		p.MarginDB = 0
	}
	if p.StabMarginDB < 0 {
		p.StabMarginDB = 0
	}
	if p.Detector == "" {
		p.Detector = "av"
	}
	if p.Standard == "" {
		p.Standard = d.Standard
	}
	if p.Topology == "" {
		p.Topology = d.Topology
	}
	if p.Mode == "ac" && p.Topology != "pfc" {
		p.Topology = "flyback"
	}
	if p.Stages == "" {
		p.Stages = "auto"
	}
	if p.CMFilter == "" {
		p.CMFilter = "auto"
	}
	return p
}
