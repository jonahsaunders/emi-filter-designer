package engine

import "math"

// LimitSeg is one frequency segment of a conducted-emission limit.  Levels
// vary linearly with log10(f) between F1 and F2.  NaN = detector not defined.
type LimitSeg struct {
	Name string  `json:"name"`
	F1   float64 `json:"f1"`
	F2   float64 `json:"f2"`
	PK1  float64 `json:"pk1"`
	PK2  float64 `json:"pk2"`
	QP1  float64 `json:"qp1"`
	QP2  float64 `json:"qp2"`
	AV1  float64 `json:"av1"`
	AV2  float64 `json:"av2"`
}

type Standard struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	LISN  string     `json:"lisn"` // "50uH" or "5uH"
	Fmin  float64    `json:"fmin"`
	Fmax  float64    `json:"fmax"`
	Segs  []LimitSeg `json:"segs"`
	Notes string     `json:"notes"`
}

var nan = math.NaN()

func flat(name string, f1, f2, pk, qp, av float64) LimitSeg {
	return LimitSeg{Name: name, F1: f1, F2: f2, PK1: pk, PK2: pk, QP1: qp, QP2: qp, AV1: av, AV2: av}
}

func slope(name string, f1, f2, qp1, qp2, av1, av2 float64) LimitSeg {
	return LimitSeg{Name: name, F1: f1, F2: f2, PK1: nan, PK2: nan, QP1: qp1, QP2: qp2, AV1: av1, AV2: av2}
}

func classB32(id, name string) Standard {
	return Standard{ID: id, Name: name, LISN: "50uH", Fmin: 150e3, Fmax: 30e6,
		Segs: []LimitSeg{
			slope("0.15-0.5 MHz", 150e3, 500e3, 66, 56, 56, 46),
			flat("0.5-5 MHz", 500e3, 5e6, nan, 56, 46),
			flat("5-30 MHz", 5e6, 30e6, nan, 60, 50),
		},
		Notes: "AC mains port, 50 µH/50 Ω V-network (CISPR 16-1-2)."}
}

func classA32(id, name string) Standard {
	return Standard{ID: id, Name: name, LISN: "50uH", Fmin: 150e3, Fmax: 30e6,
		Segs: []LimitSeg{
			flat("0.15-0.5 MHz", 150e3, 500e3, nan, 79, 66),
			flat("0.5-30 MHz", 500e3, 30e6, nan, 73, 60),
		},
		Notes: "AC mains port, 50 µH/50 Ω V-network (CISPR 16-1-2)."}
}

// CISPR 25 (voltage method) limits, class 5..1 as {peak, QP, AV}.
type c25row struct {
	name   string
	f1, f2 float64
	lv     [5][3]float64 // index 0 = class 5 ... index 4 = class 1
}

var cispr25Table = []c25row{
	{"LW", 0.15e6, 0.30e6, [5][3]float64{{70, 57, 50}, {80, 67, 60}, {90, 77, 70}, {100, 87, 80}, {110, 97, 90}}},
	{"MW", 0.53e6, 1.8e6, [5][3]float64{{54, 41, 34}, {62, 49, 42}, {70, 57, 50}, {78, 65, 58}, {86, 73, 66}}},
	{"SW", 5.9e6, 6.2e6, [5][3]float64{{53, 40, 33}, {59, 46, 39}, {65, 52, 45}, {71, 58, 51}, {77, 64, 57}}},
	{"CB", 26e6, 28e6, [5][3]float64{{44, 31, 24}, {50, 37, 30}, {56, 43, 36}, {62, 49, 42}, {68, 55, 48}}},
	{"VHF", 30e6, 54e6, [5][3]float64{{44, 31, 24}, {50, 37, 30}, {56, 43, 36}, {62, 49, 42}, {68, 55, 48}}},
	{"TV Band I", 41e6, 88e6, [5][3]float64{{34, nan, 24}, {40, nan, 30}, {46, nan, 36}, {52, nan, 42}, {58, nan, 48}}},
	{"VHF", 68e6, 87e6, [5][3]float64{{38, 25, 18}, {44, 31, 24}, {50, 37, 30}, {56, 43, 36}, {62, 49, 42}}},
	{"FM", 76e6, 108e6, [5][3]float64{{38, 25, 18}, {44, 31, 24}, {50, 37, 30}, {56, 43, 36}, {62, 49, 42}}},
}

func cispr25(class int) Standard {
	s := Standard{ID: "cispr25_c" + string(rune('0'+class)), Name: "CISPR 25 Class " + string(rune('0'+class)) + " (voltage method)",
		LISN: "5uH", Fmin: 150e3, Fmax: 108e6,
		Notes: "Automotive, 5 µH/50 Ω artificial network. Band limits per CISPR 25 conducted-voltage table; verify against the edition your OEM specifies."}
	for _, r := range cispr25Table {
		v := r.lv[5-class]
		s.Segs = append(s.Segs, flat(r.name, r.f1, r.f2, v[0], v[1], v[2]))
	}
	return s
}

// Standards is the list of supported limit sets.
var Standards = func() []Standard {
	out := []Standard{
		classB32("cispr32_b", "CISPR 32 / EN 55032 Class B"),
		classA32("cispr32_a", "CISPR 32 / EN 55032 Class A"),
		classB32("cispr11_b", "CISPR 11 / EN 55011 Group 1 Class B"),
		classA32("cispr11_a", "CISPR 11 / EN 55011 Group 1 Class A"),
		classB32("fcc15_b", "FCC Part 15 Class B (conducted)"),
		classA32("fcc15_a", "FCC Part 15 Class A (conducted)"),
	}
	for c := 5; c >= 1; c-- {
		out = append(out, cispr25(c))
	}
	return out
}()

func GetStandard(id string) Standard {
	for _, s := range Standards {
		if s.ID == id {
			return s
		}
	}
	return Standards[0]
}

func interpLog(f, f1, f2, v1, v2 float64) float64 {
	if math.IsNaN(v1) || math.IsNaN(v2) {
		return nan
	}
	if f2 <= f1 {
		return v1
	}
	t := math.Log10(f/f1) / math.Log10(f2/f1)
	return v1 + t*(v2-v1)
}

// Limit returns the (peak, QP, AV) limits at f.  When several segments
// overlap, the lowest value per detector applies.  ok=false outside all bands.
func (s Standard) Limit(f float64) (pk, qp, av float64, ok bool) {
	pk, qp, av = nan, nan, nan
	for _, g := range s.Segs {
		if f < g.F1 || f > g.F2 {
			continue
		}
		ok = true
		p := interpLog(f, g.F1, g.F2, g.PK1, g.PK2)
		q := interpLog(f, g.F1, g.F2, g.QP1, g.QP2)
		a := interpLog(f, g.F1, g.F2, g.AV1, g.AV2)
		pk = nanMin(pk, p)
		qp = nanMin(qp, q)
		av = nanMin(av, a)
	}
	return
}

// Target returns the limit used for pass/fail for the chosen detector.  For
// narrow-band switching harmonics PK, QP and AV readings are ~equal, so the
// AV limit is the governing (strictest) one.
func (s Standard) Target(f float64, det string) (float64, bool) {
	pk, qp, av, ok := s.Limit(f)
	if !ok {
		return nan, false
	}
	var v float64
	switch det {
	case "pk":
		v = firstNum(pk, qp, av)
	case "qp":
		v = firstNum(qp, pk, av)
	default:
		v = firstNum(av, qp, pk)
	}
	return v, !math.IsNaN(v)
}

func firstNum(v ...float64) float64 {
	for _, x := range v {
		if !math.IsNaN(x) {
			return x
		}
	}
	return nan
}

func nanMin(a, b float64) float64 {
	if math.IsNaN(a) {
		return b
	}
	if math.IsNaN(b) {
		return a
	}
	return math.Min(a, b)
}
