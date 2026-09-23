package engine

import (
	"fmt"
	"math"
	"sort"
)

// ---------------------------------------------------------------------------
// Circuit builder (ladder topology)

type builder struct {
	c    Circuit
	t, b int
	cnt  map[string]int
}

func newBuilder() *builder {
	bd := &builder{cnt: map[string]int{}}
	bd.c.InTop, bd.c.InBot = "T0", "B0"
	bd.c.Layout = append(bd.c.Layout, Stage{Type: "lisn"})
	return bd
}

func (bd *builder) T() string { return fmt.Sprintf("T%d", bd.t) }
func (bd *builder) B() string { return fmt.Sprintf("B%d", bd.b) }
func (bd *builder) ref(prefix string) string {
	bd.cnt[prefix]++
	return fmt.Sprintf("%s%d", prefix, bd.cnt[prefix])
}
func (bd *builder) add(c Comp) {
	if c.Qty < 1 {
		c.Qty = 1
	}
	bd.c.Comps = append(bd.c.Comps, c)
}
func (bd *builder) shunt(cs ...Comp) {
	st := Stage{Type: "shunt"}
	for _, c := range cs {
		c.Nodes = []string{bd.T(), bd.B()}
		bd.add(c)
		st.Refs = append(st.Refs, c.Ref)
	}
	bd.c.Layout = append(bd.c.Layout, st)
}

// shuntSeries adds a series string of components between the lines.
func (bd *builder) shuntSeries(cs ...Comp) {
	st := Stage{Type: "shunt_series"}
	prev := bd.T()
	for i, c := range cs {
		next := bd.B()
		if i < len(cs)-1 {
			next = "M_" + c.Ref
		}
		c.Nodes = []string{prev, next}
		prev = next
		bd.add(c)
		st.Refs = append(st.Refs, c.Ref)
	}
	bd.c.Layout = append(bd.c.Layout, st)
}
func (bd *builder) seriesTop(c Comp) {
	c.Nodes = []string{bd.T(), fmt.Sprintf("T%d", bd.t+1)}
	bd.t++
	bd.add(c)
	bd.c.Layout = append(bd.c.Layout, Stage{Type: "series_top", Refs: []string{c.Ref}})
}
func (bd *builder) seriesBoth(ct, cb Comp) {
	ct.Nodes = []string{bd.T(), fmt.Sprintf("T%d", bd.t+1)}
	cb.Nodes = []string{bd.B(), fmt.Sprintf("B%d", bd.b+1)}
	bd.t++
	bd.b++
	bd.add(ct)
	bd.add(cb)
	bd.c.Layout = append(bd.c.Layout, Stage{Type: "series_both", Refs: []string{ct.Ref, cb.Ref}})
}
func (bd *builder) cmc(c Comp) {
	c.Nodes = []string{bd.T(), fmt.Sprintf("T%d", bd.t+1), bd.B(), fmt.Sprintf("B%d", bd.b+1)}
	bd.t++
	bd.b++
	bd.add(c)
	bd.c.Layout = append(bd.c.Layout, Stage{Type: "cmc", Refs: []string{c.Ref}})
}
func (bd *builder) y(ct, cb Comp) {
	ct.Nodes = []string{bd.T(), "GND"}
	cb.Nodes = []string{bd.B(), "GND"}
	bd.add(ct)
	bd.add(cb)
	bd.c.Layout = append(bd.c.Layout, Stage{Type: "y", Refs: []string{ct.Ref, cb.Ref}})
}
func (bd *builder) finish(p Params) *Circuit {
	bd.c.OutTop, bd.c.OutBot = bd.T(), bd.B()
	cin := Comp{Ref: "CIN", Kind: "cap", Sub: "mlcc", Role: "Converter input capacitor (on converter, not in BOM)",
		Nodes: []string{bd.c.OutTop, bd.c.OutBot}, Qty: 1, Value: p.CinC, ESR: p.CinESR, ESL: p.CinESL, InBOM: false}
	if p.Mode == "ac" {
		cin.Sub = "elec"
		if p.Topology == "pfc" {
			cin.Sub = "film"
			cin.Role = "Converter input capacitor after bridge (not in BOM)"
		} else {
			cin.Role = "Bulk capacitor after bridge rectifier (not in BOM)"
		}
	} else {
		cin.Class = "X7R"
	}
	bd.c.Comps = append(bd.c.Comps, cin)
	bd.c.Layout = append(bd.c.Layout, Stage{Type: "conv", Refs: []string{"CIN"}})
	c := bd.c
	return &c
}

// ---------------------------------------------------------------------------
// Component factories

func genericPart(c *Comp) {
	kw := SearchKeywords(*c)
	c.Part = &PartInfo{Source: "generic", Description: kw, MouserURL: MouserURL(kw), DigikeyURL: DigikeyURL(kw)}
}

func fromLib(ref, role string, lp LibPart) Comp {
	c := Comp{Ref: ref, Kind: lp.Kind, Role: role, Qty: 1, InBOM: true}
	lp.ApplyTo(&c)
	return c
}

// mlccBank chooses a library MLCC and a quantity to reach an effective
// capacitance at the given DC bias.
func mlccBank(ref, role string, ceff, vreq, vbias float64, maxQty int, bulkOnly bool) Comp {
	type opt struct {
		lp    LibPart
		q     int
		score float64
	}
	var best *opt
	for _, lp := range MLCCs(vreq) {
		if bulkOnly && lp.Value < 0.9e-6 {
			continue
		}
		c := Comp{Kind: "cap", Sub: "mlcc", Value: lp.Value, VRated: lp.VRated, VBias: vbias, Class: lp.Class}
		eff := c.EffC()
		q := int(math.Ceil(ceff/eff - 1e-9))
		if q < 1 {
			q = 1
		}
		if q > maxQty {
			continue
		}
		over := float64(q)*eff/ceff - 1
		s := float64(q) + 3*over
		if best == nil || s < best.score {
			best = &opt{lp, q, s}
		}
	}
	if best != nil {
		c := fromLib(ref, role, best.lp)
		c.Qty = best.q
		c.VBias = vbias
		c.ReqV = vreq
		return c
	}
	// generic (not in library) - one part of the required value
	c := Comp{Ref: ref, Kind: "cap", Sub: "mlcc", Role: role, Qty: 1, Value: snapSeries(ceff, e6, true), VRated: stdVoltage(vreq),
		VBias: vbias, Class: "X7R", ESR: 0.005, ESL: 1.3e-9, Package: "1210", ReqV: vreq, InBOM: true}
	// compensate derating so that the effective value is met
	for i := 0; i < 8 && c.EffC() < ceff; i++ {
		c.Value = snapSeries(c.Value*1.3, e6, true)
	}
	genericPart(&c)
	return c
}

func resistor(ref, role string, r float64, pkg string) Comp {
	r = SnapE12(r)
	mpn := YageoResistorMPN(r, pkg)
	c := Comp{Ref: ref, Kind: "res", Sub: "chip", Role: role, Qty: 1, Value: r, Package: pkg, InBOM: true}
	c.Part = &PartInfo{MPN: mpn, Manufacturer: "Yageo", Description: fmt.Sprintf("Thick-film resistor %s 1%% %s", FmtEng(r, "Ω"), pkg),
		Source: "library", MouserURL: MouserURL(mpn), DigikeyURL: DigikeyURL(mpn), Verified: true}
	return c
}

func inductorComp(ref, role string, l, irms, isat float64) Comp {
	if lp, ok := PickInductor(l, irms, isat); ok {
		c := fromLib(ref, role, lp)
		c.ReqI, c.ReqIsat = irms, isat
		return c
	}
	c := Comp{Ref: ref, Kind: "ind", Sub: "power", Role: role, Qty: 1, Value: snapSeries(l, e6, true), DCR: 0.02, SRF: 0,
		ReqI: irms, ReqIsat: isat, InBOM: true}
	genericPart(&c)
	return c
}

func qOpt(n float64) float64 {
	return math.Sqrt((2 + n) * (4 + 3*n) / (2 * n * n * (4 + n)))
}

// damping leg: parallel R-Cd damping (Erickson) at the filter output.
func dampingLeg(cd, rd, vreq, vbias float64) []Comp {
	if cd >= 20e-6 {
		// electrolytic / polymer with its ESR as (part of) the damping resistance
		var cands []LibPart
		for _, lp := range Library {
			if lp.Kind == "cap" && (lp.Sub == "elec" || lp.Sub == "polymer") && lp.VRated >= vreq {
				cands = append(cands, lp)
			}
		}
		if len(cands) > 0 {
			bestI, bestQ, bestS := -1, 0, math.Inf(1)
			for i, lp := range cands {
				q := int(math.Ceil(cd/lp.Value - 0.15))
				if q < 1 {
					q = 1
				}
				if q > 3 {
					continue
				}
				esr := lp.ESR / float64(q)
				// prefer ESR close to (but below) Rd, few parts
				s := float64(q) + 2*math.Abs(math.Log((esr+1e-3)/rd))
				if esr > rd*1.4 {
					s += 10
				}
				if s < bestS {
					bestI, bestQ, bestS = i, q, s
				}
			}
			if bestI >= 0 {
				lp := cands[bestI]
				c := fromLib("CD1", "Damping capacitor (bulk)", lp)
				c.Qty = bestQ
				c.ReqV = vreq
				esr := lp.ESR / float64(bestQ)
				if rd-esr > 0.25*rd && rd-esr > 0.02 {
					r := resistor("RD1", "Damping resistor (series with CD1)", (rd-esr)*float64(bestQ), "1206")
					r.Qty = bestQ
					return []Comp{r, c}
				}
				return []Comp{c}
			}
		}
		c := Comp{Ref: "CD1", Kind: "cap", Sub: "elec", Role: "Damping capacitor (bulk)", Qty: 1, Value: snapSeries(cd, e6, true),
			VRated: stdVoltage(vreq), ESR: rd * 0.5, ESL: 5e-9, ReqV: vreq, InBOM: true}
		genericPart(&c)
		r := resistor("RD1", "Damping resistor (series with CD1)", rd*0.5, "1206")
		return []Comp{r, c}
	}
	c := mlccBank("CD1", "Damping capacitor", cd, vreq, vbias, 8, true)
	r := resistor("RD1", "Damping resistor (series with CD1)", math.Max(rd-c.ESR/float64(c.Qty), 0.01), "1206")
	return []Comp{r, c}
}

// ---------------------------------------------------------------------------
// DC design

type dcSpec struct {
	f0     float64
	stages int
	cmIdx  int // index into CM choke list, -1 = none
	dampN  float64
	dampRk float64 // multiplier on the optimum Rd
	dampCd float64 // absolute damping capacitance (overrides dampN when > 0)
	dampRd float64 // absolute damping resistance
}

type DesignOutput struct {
	Circuit *Circuit `json:"circuit"`
	Log     []string `json:"log"`
}

func dcBuild(p Params, s dcSpec, chokes []LibPart) *Circuit {
	I := InputCurrentMax(p)
	irms, isat := I*1.2, I*1.4
	vreq := p.VinAbsMax * 1.25
	vbias := p.VinNom
	zin := ConverterZin(p)
	kst := math.Pow(10, p.StabMarginDB/20)
	n := s.dampN
	if n <= 0 {
		n = 4
	}
	r0max := zin / (kst * math.Sqrt(2*(2+n)/n))
	w0 := 2 * math.Pi * s.f0
	bd := newBuilder()

	// HF bypass caps at the connector
	hf := []Comp{}
	for _, lp := range MLCCs(vreq) {
		if lp.Value == 100e-9 {
			hf = append(hf, fromLib("CHF1", "HF bypass at connector", lp))
			break
		}
	}
	if len(hf) == 0 {
		c := Comp{Ref: "CHF1", Kind: "cap", Sub: "mlcc", Role: "HF bypass at connector", Qty: 1, Value: 100e-9, VRated: stdVoltage(vreq),
			Class: "X7R", ESR: 0.02, ESL: 0.8e-9, Package: "0603", InBOM: true}
		genericPart(&c)
		hf = append(hf, c)
	}
	if GetStandard(p.Standard).Fmax > 30e6 {
		for _, lp := range MLCCs(vreq) {
			if lp.Value == 10e-9 {
				hf = append(hf, fromLib("CHF2", "VHF bypass at connector (place closest to connector)", lp))
				break
			}
		}
	}
	for i := range hf {
		hf[i].ReqV, hf[i].VBias = vreq, vbias
	}
	bd.shunt(hf...)

	// Common-mode stage
	if s.cmIdx >= 0 && s.cmIdx < len(chokes) && !singleLISN(p) {
		lp := chokes[s.cmIdx]
		c := fromLib("LCM1", "Common-mode choke", lp)
		c.Leak = lp.Value * math.Max(p.LeakRatio, 0.002)
		c.ReqI = irms
		bd.cmc(c)
		if p.AllowY {
			var yc []Comp
			for _, ref := range []string{"CY1", "CY2"} {
				cy := p.MaxCyDC
				var c Comp
				found := false
				for _, lp := range MLCCs(math.Max(vreq, 100)) {
					if math.Abs(lp.Value-cy) < cy*0.01 {
						c = fromLib(ref, "Line-to-chassis (Y) capacitor", lp)
						found = true
						break
					}
				}
				if !found {
					c = Comp{Ref: ref, Kind: "cap", Sub: "mlcc", Role: "Line-to-chassis (Y) capacitor", Qty: 1, Value: cy, VRated: stdVoltage(math.Max(vreq, 100)),
						Class: "X7R", ESR: 0.02, ESL: 0.8e-9, Package: "0603", InBOM: true}
					genericPart(&c)
				}
				c.ReqV, c.VBias = math.Max(vreq, 100), vbias
				yc = append(yc, c)
			}
			bd.y(yc[0], yc[1])
		}
	}

	// DM stage(s)
	lTarget := r0max / w0
	lmax := MaxInductor(irms, isat)
	ind := inductorComp("L1", "DM filter inductor", math.Min(lTarget, math.Max(lmax, 1e-7)), irms, isat)
	L := ind.Value
	cTarget := 1 / (w0 * w0 * L)
	cinEff := p.CinC * dcBiasFactor(Comp{Kind: "cap", Sub: "mlcc", Value: p.CinC, VRated: stdVoltage(p.VinAbsMax * 1.25), VBias: vbias})

	c1 := mlccBank("C1", "DM filter input capacitor", math.Max(cTarget*0.5, 1e-6), vreq, vbias, 6, true)
	bd.shunt(c1)
	bd.seriesTop(ind)
	if s.stages >= 2 {
		cm := mlccBank("C2", "DM filter middle capacitor", cTarget, vreq, vbias, 8, true)
		bd.shunt(cm)
		ind2 := inductorComp("L2", "DM filter inductor (2nd stage)", L, irms, isat)
		ind2.Ref = "L2"
		bd.seriesTop(ind2)
	}
	if add := cTarget - cinEff; add > 0.2e-6 {
		ref := "C2"
		if s.stages >= 2 {
			ref = "C3"
		}
		co := mlccBank(ref, "DM filter output capacitor (at converter input)", add, vreq, vbias, 8, true)
		bd.shunt(co)
	}
	// damping (Erickson optimum parallel R-Cd)
	r0 := math.Sqrt(L / cTarget)
	rk := s.dampRk
	if rk <= 0 {
		rk = 1
	}
	if s.dampCd > 0 {
		bd.shuntSeries(dampingLeg(s.dampCd, s.dampRd, vreq, vbias)...)
	} else {
		bd.shuntSeries(dampingLeg(n*cTarget, r0*qOpt(n)*rk, vreq, vbias)...)
	}

	circ := bd.finish(p)
	circ.Description = fmt.Sprintf("%d-stage DM π filter%s, f0 ≈ %s, R0 ≈ %.2f Ω",
		s.stages, map[bool]string{true: " + CM choke", false: ""}[s.cmIdx >= 0 && !singleLISN(p)], FmtEng(s.f0, "Hz"), r0)
	return circ
}

func designDC(p Params) (*Circuit, []string) {
	var log []string
	logf := func(f string, a ...any) { log = append(log, fmt.Sprintf(f, a...)) }
	I := InputCurrentMax(p)
	chokes := CMChokes(I*1.2, false)
	hs := harmonics(p, true)
	if len(hs.f) == 0 {
		logf("No switching harmonics fall inside the limit bands - only HF bypassing is needed.")
	}
	// baseline
	base := newBuilder().finish(p)
	var reqDM, reqCM []float64
	if len(hs.f) > 0 {
		dm0, cm0, tot0, _ := lisnLevels(p, base, false, hs)
		mt, md, mc, fAt := margins(hs, dm0, cm0, tot0)
		logf("Unfiltered: worst margin %.1f dB at %s (DM %.1f dB, CM %.1f dB).", mt, FmtEng(fAt, "Hz"), md, mc)
		for i := range hs.f {
			reqDM = append(reqDM, dm0[i]-(hs.tgt[i]-p.MarginDB))
			reqCM = append(reqCM, cm0[i]-(hs.tgt[i]-p.MarginDB))
		}
	}
	f0est := func(order float64) float64 {
		f0 := p.Fsw
		for i, r := range reqDM {
			if r > 0 {
				f0 = math.Min(f0, hs.f[i]/math.Pow(10, r/order))
			}
		}
		return math.Min(f0, p.Fsw*0.7)
	}
	stages := 1
	if p.Stages == "2" {
		stages = 2
	}
	spec := dcSpec{stages: stages, cmIdx: -1, dampN: 4, dampRk: 1}
	if p.CMFilter == "on" && len(chokes) > 0 && !singleLISN(p) {
		spec.cmIdx = 0
	}
	eval := func(s dcSpec) (float64, float64, float64) {
		if len(hs.f) == 0 {
			return 99, 99, 99
		}
		return quickEval(p, dcBuild(p, s, chokes), hs)
	}
	feasible := func(s dcSpec) bool {
		c := dcBuild(p, s, chokes)
		for _, cp := range c.Comps {
			if cp.Kind == "cap" && cp.Sub == "mlcc" && cp.Ref != "CD1" && cp.Value*float64(cp.Qty) > 80e-6 {
				return false // impractically large MLCC bank
			}
		}
		return true
	}

	// --- DM sizing: find the highest f0 that meets the DM target ---
	searchF0 := func(s dcSpec) (dcSpec, bool) {
		order := 40.0 * float64(s.stages)
		s.f0 = f0est(order)
		pass := func(s dcSpec) bool {
			_, md, _ := eval(s)
			return md >= p.MarginDB && feasible(s)
		}
		ok := pass(s)
		lo, hi := 0.0, 0.0
		if ok {
			lo = s.f0
			for i := 0; i < 8 && s.f0 < p.Fsw*0.7; i++ {
				s.f0 *= 1.4
				if pass(s) {
					lo = s.f0
				} else {
					hi = s.f0
					break
				}
			}
		} else {
			hi = s.f0
			for i := 0; i < 14; i++ {
				s.f0 /= 1.4
				if s.f0 < 300 {
					break
				}
				if pass(s) {
					lo = s.f0
					break
				}
				if !feasible(s) {
					break
				}
			}
		}
		if lo == 0 {
			s.f0 = hi
			return s, false
		}
		if hi > 0 {
			for i := 0; i < 5; i++ {
				m := math.Sqrt(lo * hi)
				s.f0 = m
				if pass(s) {
					lo = m
				} else {
					hi = m
				}
			}
		}
		s.f0 = lo
		return s, true
	}

	best, ok := searchF0(spec)
	if !ok && p.Stages == "auto" && spec.stages == 1 {
		logf("Single-stage filter cannot meet the DM target with practical parts - trying a 2-stage filter.")
		spec.stages = 2
		best, ok = searchF0(spec)
	} else if ok && p.Stages == "auto" {
		// a 2-stage filter can be much smaller when the corner frequency is very low
		if best.f0 < p.Fsw/40 {
			s2 := spec
			s2.stages = 2
			if b2, ok2 := searchF0(s2); ok2 && b2.f0 > best.f0*4 {
				logf("2-stage filter allows a %.1fx higher corner frequency - using 2 stages.", b2.f0/best.f0)
				best = b2
			}
		}
	}
	if !ok {
		logf("WARNING: DM target could not be met with library parts; showing the closest design.")
	}
	_, md, mc := eval(best)
	logf("DM filter: %d stage(s), f0 = %s, DM margin %.1f dB.", best.stages, FmtEng(best.f0, "Hz"), md)

	// --- CM ---
	if !singleLISN(p) && p.CMFilter != "off" && len(hs.f) > 0 && len(chokes) > 0 {
		if mc < p.MarginDB+3 || p.CMFilter == "on" {
			order := make([]int, len(chokes))
			for i := range order {
				order[i] = i
			}
			sort.SliceStable(order, func(i, j int) bool {
				return chokeSize(chokes[order[i]])+chokes[order[i]].DCR*10 < chokeSize(chokes[order[j]])+chokes[order[j]].DCR*10
			})
			bestIdx, bestM := -1, -999.0
			for _, i := range order {
				t := best
				t.cmIdx = i
				_, _, m := eval(t)
				if m > bestM {
					bestIdx, bestM = i, m
				}
				if m >= p.MarginDB+3 {
					bestIdx, bestM = i, m
					break
				}
			}
			best.cmIdx = bestIdx
			logf("CM stage: %s (%s, %g A), CM margin %.1f dB.", chokes[bestIdx].MPN, FmtEng(chokes[bestIdx].Value, "H"), chokes[bestIdx].IRms, bestM)
			if bestM < p.MarginDB {
				logf("WARNING: CM target not met with library chokes - add Y capacitance (if a chassis connection exists), reduce Cp, or search for a larger choke.")
			}
			// the choke's leakage inductance adds DM attenuation: re-optimise the DM filter
			if b2, ok2 := searchF0(best); ok2 && b2.f0 > best.f0 {
				best = b2
				_, md2, _ := eval(best)
				logf("DM filter re-sized with the CM choke present: f0 = %s, DM margin %.1f dB.", FmtEng(best.f0, "Hz"), md2)
			}
		} else {
			logf("CM emissions meet the target without a CM choke (CM margin %.1f dB).", mc)
		}
	} else if singleLISN(p) {
		logf("Return line grounded locally: CM noise does not reach the artificial network in this model; no CM choke fitted.")
	}
	// --- total (DM + CM worst-case sum) ---
	for i := 0; i < 10 && len(hs.f) > 0; i++ {
		mt, md, mc := eval(best)
		if mt >= p.MarginDB {
			break
		}
		if (mc < md) && best.cmIdx >= 0 && best.cmIdx < len(chokes)-1 {
			best.cmIdx++
		} else if (mc < md) && best.cmIdx < 0 && len(chokes) > 0 && !singleLISN(p) && p.CMFilter != "off" {
			best.cmIdx = 0
		} else {
			best.f0 /= 1.2
		}
	}
	// --- damping / stability ---
	best = tuneDamping(p, best, chokes, &log)
	// re-check emissions after damping changes
	for i := 0; i < 5 && len(hs.f) > 0; i++ {
		mt, _, _ := eval(best)
		if mt >= p.MarginDB {
			break
		}
		best.f0 /= 1.15
		best = tuneDamping(p, best, chokes, nil)
	}
	c := dcBuild(p, best, chokes)
	if len(hs.f) > 0 {
		mt, _, _ := quickEval(p, c, hs)
		logf("Final: worst total margin %.1f dB (target ≥ %.1f dB).", mt, p.MarginDB)
	}
	return c, log
}

// tuneDamping sizes the R-Cd damping leg at the converter input so that the
// filter output impedance (including the artificial network / supply
// impedance) stays below |Zin| by the requested Middlebrook margin, using
// the smallest damping capacitor that achieves it.
func tuneDamping(p Params, s dcSpec, chokes []LibPart, log *[]string) dcSpec {
	zinDB := 20 * math.Log10(ConverterZin(p))
	fc := stabilityFcheck(p)
	fs := logspace(100, fc, 70)
	margin := func(t dcSpec) float64 {
		z := zout(p, dcBuild(p, t, chokes), fs)
		m := -999.0
		for _, v := range z {
			m = math.Max(m, v)
		}
		return zinDB - m
	}
	// Erickson optimum first (cheap and usually adequate)
	t := s
	t.dampCd, t.dampRd = 0, 0
	t.dampN, t.dampRk = 4, 1
	if m := margin(t); m >= p.StabMarginDB {
		if log != nil {
			*log = append(*log, fmt.Sprintf("Damping: Erickson optimum (Cd = 4\u00d7C) \u2192 Middlebrook margin %.1f dB.", m))
		}
		return t
	}
	bestS, bestM := t, margin(t)
	for _, cd := range []float64{1e-6, 2.2e-6, 4.7e-6, 10e-6, 22e-6, 47e-6, 100e-6, 220e-6} {
		var cdBest dcSpec
		cdM := -999.0
		for _, rd := range []float64{0.03, 0.07, 0.15, 0.33, 0.7, 1.5, 3.3, 7} {
			u := s
			u.dampCd, u.dampRd = cd, rd
			if m := margin(u); m > cdM {
				cdBest, cdM = u, m
			}
		}
		// refine Rd around the best value
		for _, k := range []float64{0.7, 1.4} {
			u := cdBest
			u.dampRd *= k
			if m := margin(u); m > cdM {
				cdBest, cdM = u, m
			}
		}
		if cdM > bestM {
			bestS, bestM = cdBest, cdM
		}
		if cdM >= p.StabMarginDB {
			if log != nil {
				*log = append(*log, fmt.Sprintf("Damping: Cd = %s with Rd \u2248 %s \u2192 Middlebrook margin %.1f dB.", FmtEng(cd, "F"), FmtEng(cdBest.dampRd, "\u03a9"), cdM))
			}
			return cdBest
		}
	}
	if log != nil {
		*log = append(*log, fmt.Sprintf("WARNING: best achievable Middlebrook margin is %.1f dB (target %.1f dB). The resonance of the supply/LISN inductance with the filter capacitors needs more damping - add a larger bulk capacitor with suitable ESR or reduce the input capacitance.", bestM, p.StabMarginDB))
	}
	return bestS
}

// ---------------------------------------------------------------------------
// AC design

var xValues = []float64{0.1e-6, 0.22e-6, 0.33e-6, 0.47e-6, 1e-6}
var yValues = []float64{1e-9, 2.2e-9, 3.3e-9, 4.7e-9}
var dmLValues = []float64{4.7e-6, 10e-6, 22e-6, 33e-6, 47e-6}

type acSpec struct {
	cm1, cm2 int // choke indices (cm2 = -1: single stage)
	cx       int // X cap index
	ldm      int // DM inductor index (-1 none)
}

func xcap(ref, role string, v float64) Comp {
	for _, lp := range Library {
		if lp.Sub == "film_x2" && math.Abs(lp.Value-v) < v*0.01 {
			c := fromLib(ref, role, lp)
			c.ReqV, c.ReqVAC, c.Class = 275, true, "X2"
			return c
		}
	}
	c := Comp{Ref: ref, Kind: "cap", Sub: "film_x2", Class: "X2", Role: role, Qty: 1, Value: v, VRated: 305, ESR: 0.02, ESL: 15e-9,
		ReqV: 275, ReqVAC: true, InBOM: true}
	genericPart(&c)
	return c
}

func ycap(ref, role string, v float64) Comp {
	for _, lp := range Library {
		if lp.Sub == "cer_y" && math.Abs(lp.Value-v) < v*0.01 {
			c := fromLib(ref, role, lp)
			c.ReqV, c.ReqVAC = 250, true
			return c
		}
	}
	c := Comp{Ref: ref, Kind: "cap", Sub: "cer_y", Class: "Y1", Role: role, Qty: 1, Value: v, VRated: 250, ESR: 0.2, ESL: 6e-9,
		ReqV: 250, ReqVAC: true, InBOM: true}
	genericPart(&c)
	return c
}

func acBuild(p Params, s acSpec, chokes []LibPart, cy float64) *Circuit {
	I := InputCurrentMax(p)
	bd := newBuilder()
	cx := xValues[s.cx]
	bd.shunt(xcap("CX1", "X2 capacitor (line side)", cx))
	addChoke := func(idx int, ref string) {
		lp := chokes[idx]
		c := fromLib(ref, "Common-mode choke", lp)
		c.Leak = lp.Value * p.LeakRatio
		c.ReqI = I * 1.15
		bd.cmc(c)
	}
	addChoke(s.cm1, "LCM1")
	if s.cm2 >= 0 {
		bd.shunt(xcap("CX2", "X2 capacitor (between stages)", cx))
		addChoke(s.cm2, "LCM2")
	}
	if s.ldm >= 0 {
		l := dmLValues[s.ldm]
		ipk := math.Sqrt2 * I * 1.3
		a := inductorComp("LDM1", "DM inductor (line)", l, I*1.2, ipk)
		b := inductorComp("LDM2", "DM inductor (neutral)", l, I*1.2, ipk)
		b.Ref = "LDM2"
		bd.seriesBoth(a, b)
	}
	ref := "CX2"
	if s.cm2 >= 0 {
		ref = "CX3"
	}
	bd.shunt(xcap(ref, "X2 capacitor (converter side)", cx))
	if cy > 0 {
		bd.y(ycap("CY1", "Y1 capacitor line–PE", cy), ycap("CY2", "Y1 capacitor neutral–PE", cy))
	}
	// bleeder resistors for X capacitors (IEC 62368-1: discharge after unplugging)
	cxTot := 0.0
	for _, c := range bd.c.Comps {
		if c.Sub == "film_x2" {
			cxTot += c.Value
		}
	}
	c := bd.finish(p)
	if cxTot > 0.1e-6 {
		rTot := 1.0 / cxTot // tau = 1 s
		r := snapSeries(rTot/2, e12, false)
		if r*2 > rTot {
			r = snapSeries(rTot/2*0.82, e12, false)
		}
		r1 := resistor("RB1", "X-cap bleeder (2 in series for voltage rating)", r, "1206")
		r2 := resistor("RB2", "X-cap bleeder (2 in series for voltage rating)", r, "1206")
		r1.Nodes = []string{c.InTop, "M_RB1"}
		r2.Nodes = []string{"M_RB1", c.InBot}
		// insert right after the first X cap
		comps := []Comp{}
		for _, cp := range c.Comps {
			comps = append(comps, cp)
			if cp.Ref == "CX1" {
				comps = append(comps, r1, r2)
			}
		}
		c.Comps = comps
		lay := []Stage{c.Layout[0], c.Layout[1], {Type: "shunt_series", Refs: []string{"RB1", "RB2"}}}
		c.Layout = append(lay, c.Layout[2:]...)
		c.Notes = append(c.Notes, fmt.Sprintf("Bleeder: 2×%s in series (τ = %.2f s, %.0f mW each at %gVac). Use parts rated ≥ 200 V each or a dedicated X-cap discharge IC.",
			FmtEng(r, "Ω"), 2*r*cxTot, 1e3*p.VacMax*p.VacMax/(2*r)/2, p.VacMax))
	}
	desc := "CM choke + X/Y capacitor filter"
	if s.cm2 >= 0 {
		desc = "2-stage CM choke + X/Y capacitor filter"
	}
	if s.ldm >= 0 {
		desc += " + DM inductors"
	}
	c.Description = desc
	return c
}

// chokeSize is a rough size/cost rank used to prefer small chokes.
func chokeSize(lp LibPart) float64 {
	switch lp.Pkg {
	case "WE-CMB XS", "WE-SL5 HC":
		return 1
	case "WE-CMB S":
		return 2
	case "WE-CMB M":
		return 3
	case "WE-CMB L":
		return 4.5
	case "WE-CMB XL":
		return 7
	}
	return 3 + lp.Value*lp.IRms*lp.IRms*100
}

// subsample keeps at most n harmonics (plus the highest-level ones are
// re-checked later with the full set).
func subsample(hs harmSet, n int) harmSet {
	if len(hs.f) <= n {
		return hs
	}
	step := int(math.Ceil(float64(len(hs.f)) / float64(n)))
	var o harmSet
	for i := 0; i < len(hs.f); i += step {
		o.f = append(o.f, hs.f[i])
		o.idm = append(o.idm, hs.idm[i])
		o.vcm = append(o.vcm, hs.vcm[i])
		o.tgt = append(o.tgt, hs.tgt[i])
	}
	return o
}

func designAC(p Params) (*Circuit, []string) {
	var log []string
	logf := func(f string, a ...any) { log = append(log, fmt.Sprintf(f, a...)) }
	I := InputCurrentMax(p)
	chokes := CMChokes(I*1.15, true)
	if len(chokes) == 0 {
		chokes = []LibPart{{MPN: "", Mfr: "", Kind: "cmc", Sub: "cmc", Value: 1e-3, IRms: I * 1.2, DCR: 0.01, Desc: "Generic CM choke"}}
		logf("WARNING: no library CM choke is rated for %.2g A - using a generic 1 mH choke; search for a suitable part.", I*1.15)
	}
	logf("Line current %.2f A rms at %g Vac → CM chokes rated ≥ %.2f A considered (%d in library).", I, p.VacMin, I*1.15, len(chokes))
	// Y capacitance limited by leakage current budget
	cyMax := p.MaxCy
	if p.LeakageLimit > 0 {
		cyMax = math.Min(cyMax, p.LeakageLimit/(2*math.Pi*p.LineFreq*p.VacMax))
	}
	cy := 0.0
	for _, v := range yValues {
		if v <= cyMax*1.001 {
			cy = v
		}
	}
	if cy == 0 {
		logf("Leakage budget allows < 1 nF of Y capacitance - no Y capacitors fitted (CM attenuation relies on the choke only).")
	} else {
		logf("Y capacitors: %s per line (leakage %.2f mA at %gV/%gHz).", FmtEng(cy, "F"), 2*math.Pi*p.LineFreq*p.VacMax*cy*1e3, p.VacMax, p.LineFreq)
	}
	nx := 0
	for i, v := range xValues {
		if v <= p.MaxCx*1.001 {
			nx = i
		}
	}
	hs := harmonics(p, true)
	hsQ := subsample(hs, 120)
	if len(hs.f) == 0 {
		logf("No switching harmonics inside the limit range.")
		return acBuild(p, acSpec{cm1: 0, cm2: -1, cx: 0, ldm: -1}, chokes, cy), log
	}
	base := newBuilder().finish(p)
	dm0, cm0, tot0, _ := lisnLevels(p, base, false, hs)
	mt0, md0, mc0, fAt := margins(hs, dm0, cm0, tot0)
	logf("Unfiltered: worst margin %.1f dB at %s (DM %.1f dB, CM %.1f dB).", mt0, FmtEng(fAt, "Hz"), md0, mc0)

	evalQ := func(s acSpec) (float64, float64, float64) { return quickEval(p, acBuild(p, s, chokes, cy), hsQ) }
	evalF := func(s acSpec) (float64, float64, float64) { return quickEval(p, acBuild(p, s, chokes, cy), hs) }
	split := p.MarginDB + 3 // DM and CM each get 3 dB extra so their worst-case sum meets the margin

	// ---- 1) CM: choose the smallest choke configuration ----
	type cmCfg struct {
		a, b int
		cost float64
		m    float64
	}
	var cfgs []cmCfg
	if p.Stages != "2" {
		for i := range chokes {
			cfgs = append(cfgs, cmCfg{a: i, b: -1, cost: chokeSize(chokes[i]) + chokes[i].DCR})
		}
	}
	if p.Stages != "1" {
		for i := range chokes {
			for j := 0; j <= i; j++ {
				cfgs = append(cfgs, cmCfg{a: i, b: j, cost: 1.3*(chokeSize(chokes[i])+chokeSize(chokes[j])) + chokes[i].DCR + chokes[j].DCR})
			}
		}
	}
	sort.SliceStable(cfgs, func(i, j int) bool { return cfgs[i].cost < cfgs[j].cost })
	cxMid := nx / 2
	bestCM := -1
	bestAny := 0
	for k := range cfgs {
		_, _, mc := evalQ(acSpec{cm1: cfgs[k].a, cm2: cfgs[k].b, cx: cxMid, ldm: -1})
		cfgs[k].m = mc
		if mc > cfgs[bestAny].m || k == 0 {
			if k == 0 || mc > cfgs[bestAny].m {
				bestAny = k
			}
		}
		if mc >= split {
			bestCM = k
			break
		}
	}
	if bestCM < 0 {
		bestCM = bestAny
		logf("WARNING: no library choke combination reaches the CM target (best CM margin %.1f dB).", cfgs[bestAny].m)
	}
	s := acSpec{cm1: cfgs[bestCM].a, cm2: cfgs[bestCM].b, cx: 0, ldm: -1}
	if s.cm2 >= 0 {
		logf("CM: 2 stages %s + %s.", chokes[s.cm1].MPN, chokes[s.cm2].MPN)
	} else {
		logf("CM: %s (%s, %g A).", chokes[s.cm1].MPN, FmtEng(chokes[s.cm1].Value, "H"), chokes[s.cm1].IRms)
	}

	// ---- 2) DM: smallest X capacitors / DM inductors ----
	type dmCfg struct {
		cx, ldm int
		cost    float64
	}
	var dcfg []dmCfg
	for cx := 0; cx <= nx; cx++ {
		for l := -1; l < len(dmLValues); l++ {
			dcfg = append(dcfg, dmCfg{cx, l, float64(cx) + 2.5*float64(l+1)})
		}
	}
	sort.SliceStable(dcfg, func(i, j int) bool { return dcfg[i].cost < dcfg[j].cost })
	found := false
	bestDM, bestDMm := dcfg[len(dcfg)-1], -999.0
	for _, d := range dcfg {
		t := s
		t.cx, t.ldm = d.cx, d.ldm
		_, md, _ := evalQ(t)
		if md > bestDMm {
			bestDM, bestDMm = d, md
		}
		if md >= split {
			s, found = t, true
			break
		}
	}
	if !found {
		s.cx, s.ldm = bestDM.cx, bestDM.ldm
		logf("WARNING: DM target not reached with library X capacitors / DM inductors (best DM margin %.1f dB).", bestDMm)
	}

	// ---- 3) total, verified with every harmonic ----
	for it := 0; it < 12; it++ {
		mt, md, mc := evalF(s)
		if mt >= p.MarginDB {
			break
		}
		if md <= mc {
			if s.cx < nx {
				s.cx++
			} else if s.ldm < len(dmLValues)-1 {
				s.ldm++
			} else {
				break
			}
		} else {
			// next more expensive CM configuration with a better CM margin
			moved := false
			for k := bestCM + 1; k < len(cfgs); k++ {
				t := s
				t.cm1, t.cm2 = cfgs[k].a, cfgs[k].b
				if _, _, m2 := evalQ(t); m2 > mc+1 {
					s, bestCM, moved = t, k, true
					break
				}
			}
			if !moved {
				break
			}
		}
	}
	// ---- 4) greedy down-sizing with the full harmonic set ----
	if m, _, _ := evalF(s); m >= p.MarginDB {
		for changed := true; changed; {
			changed = false
			var trials []acSpec
			if s.ldm >= 0 {
				t := s
				t.ldm--
				trials = append(trials, t)
			}
			if s.cx > 0 {
				t := s
				t.cx--
				trials = append(trials, t)
			}
			if s.cm2 >= 0 {
				t := s
				t.cm2 = -1
				trials = append(trials, t)
				for k := range chokes {
					if chokeSize(chokes[k]) < chokeSize(chokes[s.cm2]) {
						t := s
						t.cm2 = k
						trials = append(trials, t)
					}
				}
			}
			for k := range chokes {
				if chokeSize(chokes[k]) < chokeSize(chokes[s.cm1]) || (chokeSize(chokes[k]) == chokeSize(chokes[s.cm1]) && chokes[k].Value < chokes[s.cm1].Value) {
					t := s
					t.cm1 = k
					trials = append(trials, t)
				}
			}
			for _, t := range trials {
				if m, _, _ := evalF(t); m >= p.MarginDB {
					s, changed = t, true
					break
				}
			}
		}
	}
	c := acBuild(p, s, chokes, cy)
	mt, md, mc := quickEval(p, c, hs)
	if mt < p.MarginDB {
		logf("WARNING: target not met with library parts - showing the closest design. Consider a larger CM choke from the distributor search, more Y capacitance (if leakage allows) or a lower Cp (layout/shielding).")
	}
	logf("Final: worst total margin %.1f dB (DM %.1f dB, CM %.1f dB), target ≥ %.1f dB.", mt, md, mc, p.MarginDB)
	return c, log
}

// Design runs the automatic filter synthesis.
func Design(p Params) (*DesignOutput, error) {
	p = Sanitize(p)
	var c *Circuit
	var log []string
	if p.Mode == "ac" {
		c, log = designAC(p)
	} else {
		c, log = designDC(p)
	}
	for i := range c.Comps {
		if c.Comps[i].Part == nil && c.Comps[i].InBOM {
			genericPart(&c.Comps[i])
		}
	}
	return &DesignOutput{Circuit: c, Log: log}, nil
}
