package engine

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// LibPart is an entry of the built-in (offline) component library.  Values
// are taken from manufacturer datasheets / catalog pages; always confirm
// against the current datasheet before release.
type LibPart struct {
	MPN    string  `json:"mpn"`
	Mfr    string  `json:"mfr"`
	Kind   string  `json:"kind"` // cap | ind | cmc | res
	Sub    string  `json:"sub"`
	Pkg    string  `json:"pkg"`
	Value  float64 `json:"value"`
	VRated float64 `json:"vRated"`
	VAC    bool    `json:"vac"`
	Class  string  `json:"class"`
	IRms   float64 `json:"iRms"`
	ISat   float64 `json:"iSat"`
	DCR    float64 `json:"dcr"`
	SRF    float64 `json:"srf"`
	ESR    float64 `json:"esr"`
	ESL    float64 `json:"esl"`
	Desc   string  `json:"desc"`
}

func MouserURL(q string) string {
	return "https://www.mouser.com/c/?q=" + url.QueryEscape(q)
}
func DigikeyURL(q string) string {
	return "https://www.digikey.com/en/products/result?keywords=" + url.QueryEscape(q)
}

// mounted ESL (part + pads/vias) by package
var mlccESL = map[string]float64{"0402": 0.7e-9, "0603": 0.8e-9, "0805": 1.0e-9, "1206": 1.2e-9, "1210": 1.3e-9, "2220": 1.6e-9}

func mlcc(mpn string, c, v float64, pkg, cls string) LibPart {
	esr := 0.005
	if c < 1e-6 {
		esr = 0.02
	}
	return LibPart{MPN: mpn, Mfr: "Murata", Kind: "cap", Sub: "mlcc", Pkg: pkg, Value: c, VRated: v, Class: cls,
		ESR: esr, ESL: mlccESL[pkg], Desc: fmt.Sprintf("MLCC %s %gV %s %s", FmtEng(c, "F"), v, cls, pkg)}
}

func xal(mpn string, l, dcr, isat, irms, srf float64) LibPart {
	pkg := strings.SplitN(mpn, "-", 2)[0]
	return LibPart{MPN: mpn, Mfr: "Coilcraft", Kind: "ind", Sub: "power", Pkg: pkg, Value: l * 1e-6, DCR: dcr * 1e-3,
		ISat: isat, IRms: irms, SRF: srf * 1e6, Desc: fmt.Sprintf("Shielded power inductor %s, Isat %gA, Irms %gA", FmtEng(l*1e-6, "H"), isat, irms)}
}

func wecmb(mpn, size string, lmh, irms, dcrm float64) LibPart {
	// SRF of mains CM chokes is typically 0.5..5 MHz; estimate from winding
	// capacitance ~ 8-20 pF depending on size.
	cw := map[string]float64{"XS": 6e-12, "S": 8e-12, "M": 10e-12, "L": 14e-12, "XL": 18e-12}[size]
	l := lmh * 1e-3
	return LibPart{MPN: mpn, Mfr: "Würth Elektronik", Kind: "cmc", Sub: "cmc", Pkg: "WE-CMB " + size, Value: l,
		IRms: irms, DCR: dcrm * 1e-3, VRated: 250, VAC: true, SRF: 1 / (2 * math.Pi * math.Sqrt(l*cw)),
		Desc: fmt.Sprintf("Common-mode power-line choke %gmH %gA (WE-CMB %s)", lmh, irms, size)}
}

var Library = []LibPart{
	// ---- MLCC (Murata GRM, X7R/X5R) ----
	mlcc("GRM155R71H103KA88D", 10e-9, 50, "0402", "X7R"),
	mlcc("GRM188R71H104KA93D", 100e-9, 50, "0603", "X7R"),
	mlcc("GRM188R72A104KA35D", 100e-9, 100, "0603", "X7R"),
	mlcc("GRM21BR71H105KA12L", 1e-6, 50, "0805", "X7R"),
	mlcc("GRM31CR71H475KA12L", 4.7e-6, 50, "1206", "X7R"),
	mlcc("GRM32ER71H106KA12L", 10e-6, 50, "1210", "X7R"),
	mlcc("GRM31CR61E106KA12L", 10e-6, 25, "1206", "X5R"),
	mlcc("GRM31CR72A105KA01L", 1e-6, 100, "1206", "X7R"),
	mlcc("GRM32ER72A225KA35L", 2.2e-6, 100, "1210", "X7R"),

	// ---- Aluminium electrolytic / hybrid polymer (damping) ----
	{MPN: "EEH-ZA1H101P", Mfr: "Panasonic", Kind: "cap", Sub: "polymer", Pkg: "SMD 8x10.2", Value: 100e-6, VRated: 50, ESR: 0.028, ESL: 3e-9, Desc: "Hybrid polymer Al 100µF 50V (ZA)"},
	{MPN: "EEE-FK1H101P", Mfr: "Panasonic", Kind: "cap", Sub: "elec", Pkg: "SMD 10x10.2", Value: 100e-6, VRated: 50, ESR: 0.3, ESL: 4e-9, Desc: "Al electrolytic 100µF 50V (FK)"},
	{MPN: "EEE-FK1V470P", Mfr: "Panasonic", Kind: "cap", Sub: "elec", Pkg: "SMD 8x10.2", Value: 47e-6, VRated: 35, ESR: 0.34, ESL: 4e-9, Desc: "Al electrolytic 47µF 35V (FK)"},

	// ---- X2 film capacitors (TDK/EPCOS B3292x, 305 VAC) ----
	{MPN: "B32922C3104M000", Mfr: "TDK", Kind: "cap", Sub: "film_x2", Pkg: "Radial P15", Value: 0.1e-6, VRated: 305, VAC: true, Class: "X2", ESR: 0.03, ESL: 12e-9, Desc: "X2 film 0.1µF 305VAC"},
	{MPN: "B32922C3224M000", Mfr: "TDK", Kind: "cap", Sub: "film_x2", Pkg: "Radial P15", Value: 0.22e-6, VRated: 305, VAC: true, Class: "X2", ESR: 0.025, ESL: 12e-9, Desc: "X2 film 0.22µF 305VAC"},
	{MPN: "B32922C3334M000", Mfr: "TDK", Kind: "cap", Sub: "film_x2", Pkg: "Radial P15", Value: 0.33e-6, VRated: 305, VAC: true, Class: "X2", ESR: 0.02, ESL: 13e-9, Desc: "X2 film 0.33µF 305VAC"},
	{MPN: "B32922C3474M000", Mfr: "TDK", Kind: "cap", Sub: "film_x2", Pkg: "Radial P15", Value: 0.47e-6, VRated: 305, VAC: true, Class: "X2", ESR: 0.018, ESL: 14e-9, Desc: "X2 film 0.47µF 305VAC"},
	{MPN: "B32923C3105M000", Mfr: "TDK", Kind: "cap", Sub: "film_x2", Pkg: "Radial P22.5", Value: 1e-6, VRated: 305, VAC: true, Class: "X2", ESR: 0.012, ESL: 18e-9, Desc: "X2 film 1µF 305VAC"},

	// ---- Y1 ceramic disc capacitors (Murata DE1, 250 VAC) ----
	{MPN: "DE1E3KX102MA4BN01F", Mfr: "Murata", Kind: "cap", Sub: "cer_y", Pkg: "Radial P10", Value: 1e-9, VRated: 250, VAC: true, Class: "Y1", ESR: 0.2, ESL: 6e-9, Desc: "Y1 safety ceramic 1000pF 250VAC"},
	{MPN: "DE1E3KX222MA4BN01F", Mfr: "Murata", Kind: "cap", Sub: "cer_y", Pkg: "Radial P10", Value: 2.2e-9, VRated: 250, VAC: true, Class: "Y1", ESR: 0.2, ESL: 6e-9, Desc: "Y1 safety ceramic 2200pF 250VAC"},
	{MPN: "DE1E3KX332MA5BA01F", Mfr: "Murata", Kind: "cap", Sub: "cer_y", Pkg: "Radial P10", Value: 3.3e-9, VRated: 250, VAC: true, Class: "Y1", ESR: 0.2, ESL: 6e-9, Desc: "Y1 safety ceramic 3300pF 250VAC"},
	{MPN: "DE1E3KX472MA5BA01F", Mfr: "Murata", Kind: "cap", Sub: "cer_y", Pkg: "Radial P10", Value: 4.7e-9, VRated: 250, VAC: true, Class: "Y1", ESR: 0.2, ESL: 6e-9, Desc: "Y1 safety ceramic 4700pF 250VAC"},

	// ---- Power inductors (Coilcraft XAL, DCR typ mOhm, Isat, Irms, SRF MHz) ----
	xal("XAL4020-102MEC", 1.0, 13.3, 8.7, 6.7, 79),
	xal("XAL4020-222MEC", 2.2, 35.2, 5.6, 4.0, 52),
	xal("XAL4030-332MEC", 3.3, 26.0, 5.9, 5.0, 43),
	xal("XAL4030-472MEC", 4.7, 40.1, 4.6, 3.9, 36),
	xal("XAL4030-682MEC", 6.8, 67.4, 3.6, 3.0, 29),
	xal("XAL4040-103MEC", 10, 84.0, 3.0, 2.2, 24),
	xal("XAL4040-153MEC", 15, 109, 2.9, 2.0, 20),
	xal("XAL6030-102MEC", 1.0, 5.6, 23, 13, 50),
	xal("XAL6030-222MEC", 2.2, 12.7, 15.9, 7.0, 30),
	xal("XAL6030-332MEC", 3.3, 19.9, 12.2, 6.0, 26),
	xal("XAL6060-472MEC", 4.7, 13.1, 10.5, 8.0, 21),
	xal("XAL6060-682MEC", 6.8, 18.9, 9.2, 7.0, 18),
	xal("XAL6060-103MEC", 10, 27.0, 7.6, 5.0, 14),
	xal("XAL6060-153MEC", 15, 39.8, 5.8, 4.5, 11),
	xal("XAL6060-223MEC", 22, 55.1, 5.6, 3.6, 9),
	xal("XAL6060-333MEC", 33, 95.7, 3.7, 2.7, 7),
	xal("XAL7070-102MEC", 1.0, 2.6, 34.8, 25, 64),
	xal("XAL7070-222MEC", 2.2, 5.7, 19.6, 17.8, 35),
	xal("XAL7070-332MEC", 3.3, 8.6, 19.4, 15.1, 32),
	xal("XAL7070-472MEC", 4.7, 13.0, 15.2, 13.6, 26),
	xal("XAL7070-682MEC", 6.8, 17.8, 12.8, 9.2, 20),
	xal("XAL7070-103MEC", 10, 17.5, 7.5, 9.3, 11.5),
	xal("XAL7070-153MEC", 15, 25.7, 7.0, 7.4, 9.7),
	xal("XAL7070-223MEC", 22, 34.5, 5.6, 6.5, 8.1),
	xal("XAL7070-333MEC", 33, 54.0, 4.4, 4.9, 6.7),
	xal("XAL7070-473MEC", 47, 84.4, 4.2, 4.1, 5.7),
	xal("XAL1010-102MED", 1.0, 1.0, 55, 32, 42),
	xal("XAL1010-222MED", 2.2, 2.6, 34, 24.5, 22),
	xal("XAL1010-472MED", 4.7, 5.2, 25.4, 17.5, 19),
	xal("XAL1010-682MED", 6.8, 8.1, 21.8, 14, 14),
	xal("XAL1010-103MED", 10, 13.4, 17.5, 11.5, 11),
	xal("XAL1010-153MED", 15, 16.9, 15.5, 9.9, 9),

	// ---- Common-mode chokes, mains (Wurth WE-CMB) ----
	wecmb("744821039", "XS", 39, 0.3, 3000),
	wecmb("744821120", "XS", 20, 0.5, 1000),
	wecmb("744821110", "XS", 10, 0.7, 350),
	wecmb("744821150", "XS", 5, 1, 220),
	wecmb("744821240", "XS", 4, 1.5, 140),
	wecmb("744821201", "XS", 1, 2, 45),
	wecmb("744822120", "S", 20, 0.5, 540),
	wecmb("744822110", "S", 10, 1, 360),
	wecmb("744822233", "S", 3.3, 1.5, 120),
	wecmb("744822222", "S", 2.2, 2, 70),
	wecmb("744822301", "S", 1, 3, 35),
	wecmb("744823220", "M", 20, 1.5, 270),
	wecmb("744823210", "M", 10, 2, 125),
	wecmb("744823305", "M", 5, 2.5, 95),
	wecmb("744823333", "M", 3.3, 2.5, 60),
	wecmb("744823422", "M", 2.2, 4, 30),
	wecmb("744823601", "M", 1, 6, 13),
	wecmb("744824220", "L", 20, 2, 220),
	wecmb("744824310", "L", 10, 3, 105),
	wecmb("744824407", "L", 7, 3.5, 80),
	wecmb("744824405", "L", 5, 4, 50),
	wecmb("744824433", "L", 3.3, 4, 35),
	wecmb("744824622", "L", 2.2, 6, 20),
	wecmb("744824801", "L", 1, 7.5, 10),
	wecmb("7448251201", "XL", 1, 12, 9),

	// ---- Common-mode chokes, DC / low voltage (Wurth WE-SL5 HC) ----
	{MPN: "744273501", Mfr: "Würth Elektronik", Kind: "cmc", Sub: "cmc", Pkg: "WE-SL5 HC", Value: 5e-6, IRms: 5, DCR: 0.0055, VRated: 0, SRF: 60e6, Desc: "SMD common-mode line filter 5µH 5A"},
	{MPN: "744273801", Mfr: "Würth Elektronik", Kind: "cmc", Sub: "cmc", Pkg: "WE-SL5 HC", Value: 9e-6, IRms: 3.5, DCR: 0.011, VRated: 0, SRF: 45e6, Desc: "SMD common-mode line filter 9µH 3.5A"},
	{MPN: "744273102", Mfr: "Würth Elektronik", Kind: "cmc", Sub: "cmc", Pkg: "WE-SL5 HC", Value: 11e-6, IRms: 2.5, DCR: 0.03, VRated: 0, SRF: 40e6, Desc: "SMD common-mode line filter 11µH 2.5A"},
	{MPN: "744273222", Mfr: "Würth Elektronik", Kind: "cmc", Sub: "cmc", Pkg: "WE-SL5 HC", Value: 30e-6, IRms: 1.4, DCR: 0.06, VRated: 0, SRF: 25e6, Desc: "SMD common-mode line filter 30µH 1.4A"},
}

// ---------------------------------------------------------------------------
// Standard value series

var e6 = []float64{1.0, 1.5, 2.2, 3.3, 4.7, 6.8}
var e12 = []float64{1.0, 1.2, 1.5, 1.8, 2.2, 2.7, 3.3, 3.9, 4.7, 5.6, 6.8, 8.2}

func snapSeries(x float64, series []float64, up bool) float64 {
	if x <= 0 {
		return x
	}
	dec := math.Floor(math.Log10(x))
	base := math.Pow(10, dec)
	var cands []float64
	for d := -1.0; d <= 1; d++ {
		for _, s := range series {
			cands = append(cands, s*base*math.Pow(10, d))
		}
	}
	sort.Float64s(cands)
	best := cands[0]
	for _, c := range cands {
		if up {
			if c >= x*0.999 {
				return c
			}
		} else if math.Abs(math.Log(c/x)) < math.Abs(math.Log(best/x)) {
			best = c
		}
	}
	return best
}

// SnapE12 returns the nearest E12 value.
func SnapE12(x float64) float64 { return snapSeries(x, e12, false) }

// YageoResistorMPN builds a Yageo RC-series thick-film chip resistor part
// number (1% tolerance) for the given value and package.
func YageoResistorMPN(r float64, pkg string) string {
	code := ""
	switch {
	case r >= 1e6:
		code = trimNum(r/1e6, "M")
	case r >= 1e3:
		code = trimNum(r/1e3, "K")
	default:
		code = trimNum(r, "R")
	}
	if r < 1 {
		return fmt.Sprintf("RL%sFR-07%sL", pkg, code) // Yageo RL low-ohm series
	}
	return fmt.Sprintf("RC%sFR-07%sL", pkg, code)
}

func trimNum(v float64, letter string) string {
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
	if strings.Contains(s, ".") {
		return strings.Replace(s, ".", letter, 1)
	}
	return s + letter
}

// FmtEng formats a value with an SI prefix.
func FmtEng(v float64, unit string) string {
	if v == 0 {
		return "0" + unit
	}
	prefixes := []struct {
		m float64
		p string
	}{{1e9, "G"}, {1e6, "M"}, {1e3, "k"}, {1, ""}, {1e-3, "m"}, {1e-6, "µ"}, {1e-9, "n"}, {1e-12, "p"}}
	a := math.Abs(v)
	for _, p := range prefixes {
		if a >= p.m*0.9995 {
			return strconv.FormatFloat(v/p.m, 'g', 3, 64) + p.p + unit
		}
	}
	return fmt.Sprintf("%.3g%s", v, unit)
}

// ---------------------------------------------------------------------------
// Library queries

// PickInductor returns the smallest-inductance library inductor with L >=
// lmin and adequate current ratings, preferring low DCR among equal L.
func PickInductor(lmin, irms, isat float64) (LibPart, bool) {
	var best LibPart
	found := false
	for _, p := range Library {
		if p.Kind != "ind" || p.IRms < irms || p.ISat < isat || p.Value < lmin*0.98 {
			continue
		}
		if !found || p.Value < best.Value*0.98 || (math.Abs(p.Value-best.Value) < best.Value*0.02 && p.DCR < best.DCR) {
			best, found = p, true
		}
	}
	return best, found
}

// MaxInductor returns the largest library inductance for the current.
func MaxInductor(irms, isat float64) float64 {
	m := 0.0
	for _, p := range Library {
		if p.Kind == "ind" && p.IRms >= irms && p.ISat >= isat {
			m = math.Max(m, p.Value)
		}
	}
	return m
}

// CMChokes returns library CM chokes with sufficient current (and voltage
// class) sorted by inductance.
func CMChokes(irms float64, mains bool) []LibPart {
	var out []LibPart
	for _, p := range Library {
		if p.Kind != "cmc" || p.IRms < irms {
			continue
		}
		if mains && !p.VAC {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if math.Abs(out[i].Value-out[j].Value) < out[i].Value*1e-3 {
			return out[i].IRms < out[j].IRms // smallest part that carries the current
		}
		return out[i].Value < out[j].Value
	})
	// keep the smallest qualifying choke for each inductance value
	var ded []LibPart
	for _, p := range out {
		if len(ded) > 0 && math.Abs(ded[len(ded)-1].Value-p.Value) < p.Value*1e-3 {
			continue
		}
		ded = append(ded, p)
	}
	return ded
}

// MLCCs returns library MLCCs with voltage rating >= v.
func MLCCs(v float64) []LibPart {
	var out []LibPart
	for _, p := range Library {
		if p.Kind == "cap" && p.Sub == "mlcc" && p.VRated >= v {
			out = append(out, p)
		}
	}
	return out
}

func libByMPN(mpn string) (LibPart, bool) {
	for _, p := range Library {
		if p.MPN == mpn {
			return p, true
		}
	}
	return LibPart{}, false
}

// ToComp converts a library part into a Comp (keeping role/nodes from base).
func (p LibPart) ApplyTo(c *Comp) {
	c.Value = p.Value
	if p.ESR > 0 {
		c.ESR = p.ESR
	}
	if p.ESL > 0 {
		c.ESL = p.ESL
	}
	if p.DCR > 0 {
		c.DCR = p.DCR
	}
	if p.SRF > 0 {
		c.SRF = p.SRF
	}
	if p.VRated > 0 {
		c.VRated = p.VRated
	}
	if p.Class != "" {
		c.Class = p.Class
	}
	c.Sub = p.Sub
	c.Package = p.Pkg
	c.Part = &PartInfo{MPN: p.MPN, Manufacturer: p.Mfr, Description: p.Desc, Source: "library",
		MouserURL: MouserURL(p.MPN), DigikeyURL: DigikeyURL(p.MPN), Verified: true}
}

// Candidates returns library parts that satisfy the requirements of a comp.
func Candidates(c Comp) []LibPart {
	var out []LibPart
	for _, p := range Library {
		if p.Kind != c.Kind {
			continue
		}
		switch c.Kind {
		case "cap":
			if c.ReqVAC != p.VAC {
				continue
			}
			if c.Class != "" && strings.HasPrefix(c.Class, "Y") && !strings.HasPrefix(p.Class, "Y") {
				continue
			}
			if c.Class == "X2" && p.Class != "X2" {
				continue
			}
			if p.VRated < c.ReqV {
				continue
			}
			if r := p.Value / c.Value; r < 0.45 || r > 2.2 {
				continue
			}
		case "ind", "cmc":
			if p.IRms < c.ReqI || p.ISat < c.ReqIsat {
				continue
			}
			if r := p.Value / c.Value; r < 0.6 || r > 2.5 {
				continue
			}
		default:
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		di := math.Abs(math.Log(out[i].Value / c.Value))
		dj := math.Abs(math.Log(out[j].Value / c.Value))
		if math.Abs(di-dj) > 1e-3 {
			return di < dj
		}
		return out[i].DCR < out[j].DCR
	})
	return out
}

// SearchKeywords builds a distributor keyword query for a comp.
func SearchKeywords(c Comp) string {
	v := func(x float64, u string) string { return strings.ReplaceAll(FmtEng(x, u), "µ", "u") }
	switch c.Kind {
	case "cap":
		switch c.Sub {
		case "film_x2":
			return fmt.Sprintf("X2 capacitor %s 305VAC", v(c.Value, "F"))
		case "cer_y":
			return fmt.Sprintf("Y1 capacitor %s", v(c.Value, "F"))
		case "elec", "polymer":
			return fmt.Sprintf("aluminum capacitor %s %gV", v(c.Value, "F"), stdVoltage(c.ReqV))
		default:
			return fmt.Sprintf("%s %gV X7R %s", v(c.Value, "F"), stdVoltage(c.ReqV), c.Package)
		}
	case "ind":
		return fmt.Sprintf("%s shielded power inductor", v(c.Value, "H"))
	case "cmc":
		if c.Value >= 100e-6 {
			return fmt.Sprintf("common mode choke power line %s", v(c.Value, "H"))
		}
		return fmt.Sprintf("common mode choke %s", v(c.Value, "H"))
	case "res":
		return fmt.Sprintf("resistor %s %s 1%%", v(c.Value, "Ohm"), c.Package)
	}
	return c.Role
}

var stdVoltages = []float64{6.3, 10, 16, 25, 35, 50, 63, 100, 200, 250, 450, 630, 1000}

func stdVoltage(v float64) float64 {
	for _, s := range stdVoltages {
		if s >= v {
			return s
		}
	}
	return v
}

// LibByMPN looks up a library part.
func LibByMPN(mpn string) (LibPart, bool) { return libByMPN(mpn) }

// MLCCMountedESL returns a typical mounted ESL for an MLCC case size.
func MLCCMountedESL(pkg string) (float64, bool) {
	for k, v := range mlccESL {
		if strings.Contains(pkg, k) {
			return v, true
		}
	}
	return 0, false
}
