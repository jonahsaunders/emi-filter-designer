package engine

import (
	"encoding/json"
	"math"
	"strconv"
)

// JF is a float64 that marshals NaN/Inf as JSON null.
type JF float64

func (f JF) MarshalJSON() ([]byte, error) {
	v := float64(f)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatFloat(v, 'g', 7, 64)), nil
}

func (f *JF) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*f = JF(math.NaN())
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*f = JF(v)
	return nil
}

func jfs(v []float64) []JF {
	out := make([]JF, len(v))
	for i, x := range v {
		out[i] = JF(x)
	}
	return out
}

func (s LimitSeg) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"name": s.Name, "f1": s.F1, "f2": s.F2,
		"pk1": JF(s.PK1), "pk2": JF(s.PK2), "qp1": JF(s.QP1), "qp2": JF(s.QP2),
		"av1": JF(s.AV1), "av2": JF(s.AV2),
	})
}

// Params describes the converter, the noise model, the standard and the
// design preferences.
type Params struct {
	Mode         string  `json:"mode"`     // "dc" | "ac"
	Standard     string  `json:"standard"` // see Standards
	Detector     string  `json:"detector"` // "av" | "qp" | "pk"
	MarginDB     float64 `json:"marginDb"`
	StabMarginDB float64 `json:"stabMarginDb"`

	Topology    string  `json:"topology"` // dc: buck|boost|buckboost|flyback ; ac: flyback|pfc
	Fsw         float64 `json:"fsw"`
	RiseTime    float64 `json:"riseTime"`
	Cp          float64 `json:"cp"`  // switch node -> chassis/PE parasitic capacitance
	Eff         float64 `json:"eff"` // 0..1
	Pout        float64 `json:"pout"`
	Vout        float64 `json:"vout"`
	Vrefl       float64 `json:"vrefl"`       // flyback reflected voltage
	RippleRatio float64 `json:"rippleRatio"` // boost/PFC inductor ripple (p-p / avg)

	// DC input
	VinMin    float64 `json:"vinMin"`
	VinNom    float64 `json:"vinNom"`
	VinMax    float64 `json:"vinMax"`
	VinAbsMax float64 `json:"vinAbsMax"` // transient max, used for voltage ratings

	ReturnGrounded bool    `json:"returnGrounded"` // DC: negative line grounded locally (single LISN)
	AllowY         bool    `json:"allowY"`         // DC: chassis available for Y (line-chassis) caps
	MaxCyDC        float64 `json:"maxCyDc"`

	// AC input
	VacMin       float64 `json:"vacMin"`
	VacMax       float64 `json:"vacMax"`
	LineFreq     float64 `json:"lineFreq"`
	PF           float64 `json:"pf"`
	LeakageLimit float64 `json:"leakageLimit"` // A (touch/protective-conductor current budget for Y caps)
	MaxCy        float64 `json:"maxCy"`
	MaxCx        float64 `json:"maxCx"`
	LeakRatio    float64 `json:"leakRatio"` // CM choke leakage (DM) inductance / L_cm

	// Converter input capacitor (part of converter, not in BOM)
	CinC   float64 `json:"cinC"`
	CinESR float64 `json:"cinEsr"`
	CinESL float64 `json:"cinEsl"`

	Stages   string `json:"stages"`   // auto | 1 | 2
	CMFilter string `json:"cmFilter"` // auto | on | off
}

func DefaultParams(mode string) Params {
	if mode == "ac" {
		return Params{Mode: "ac", Standard: "cispr32_b", Detector: "av", MarginDB: 6, StabMarginDB: 6,
			Topology: "flyback", Fsw: 100e3, RiseTime: 30e-9, Cp: 10e-12, Eff: 0.88, Pout: 60, Vout: 12,
			Vrefl: 100, RippleRatio: 0.3, VacMin: 90, VacMax: 264, LineFreq: 50, PF: 0.55,
			LeakageLimit: 0.5e-3, MaxCy: 4.7e-9, MaxCx: 1e-6, LeakRatio: 0.01,
			CinC: 47e-6, CinESR: 0.5, CinESL: 20e-9, Stages: "auto", CMFilter: "auto",
			AllowY: true, MaxCyDC: 100e-9, VinMin: 9, VinNom: 13.5, VinMax: 16, VinAbsMax: 40}
	}
	return Params{Mode: "dc", Standard: "cispr25_c5", Detector: "av", MarginDB: 6, StabMarginDB: 6,
		Topology: "buck", Fsw: 400e3, RiseTime: 10e-9, Cp: 2e-12, Eff: 0.9, Pout: 24, Vout: 5,
		Vrefl: 50, RippleRatio: 0.3, VinMin: 9, VinNom: 13.5, VinMax: 16, VinAbsMax: 40,
		ReturnGrounded: false, AllowY: true, MaxCyDC: 100e-9,
		VacMin: 90, VacMax: 264, LineFreq: 50, PF: 0.55, LeakageLimit: 0.5e-3, MaxCy: 4.7e-9, MaxCx: 1e-6, LeakRatio: 0.01,
		CinC: 10e-6, CinESR: 5e-3, CinESL: 1.5e-9, Stages: "auto", CMFilter: "auto"}
}

// PartInfo is the concrete part selected for a BOM line.
type PartInfo struct {
	MPN          string  `json:"mpn"`
	Manufacturer string  `json:"manufacturer"`
	Description  string  `json:"description"`
	Source       string  `json:"source"` // library | mouser | digikey | generic
	UnitPrice    float64 `json:"unitPrice"`
	Currency     string  `json:"currency"`
	Stock        int     `json:"stock"`
	MouserURL    string  `json:"mouserUrl"`
	DigikeyURL   string  `json:"digikeyUrl"`
	DatasheetURL string  `json:"datasheetUrl"`
	DistPN       string  `json:"distPn"`
	Verified     bool    `json:"verified"` // parameters taken from a datasheet/API
}

// Comp is a filter component (one BOM line; Qty identical parts in parallel).
type Comp struct {
	Ref     string   `json:"ref"`
	Kind    string   `json:"kind"` // cap | ind | res | cmc
	Sub     string   `json:"sub"`  // mlcc | elec | polymer | film_x2 | cer_y | power | cmc | chip
	Role    string   `json:"role"`
	Nodes   []string `json:"nodes"`
	Qty     int      `json:"qty"`
	Value   float64  `json:"value"` // F, H or ohm (per piece). CMC: inductance per winding
	ESR     float64  `json:"esr"`
	ESL     float64  `json:"esl"`
	DCR     float64  `json:"dcr"` // per winding for CMC
	SRF     float64  `json:"srf"` // Hz (0 = ideal)
	Qsrf    float64  `json:"qsrf"`
	Leak    float64  `json:"leak"`    // CMC: total DM leakage inductance
	VRated  float64  `json:"vRated"`  // part voltage rating
	VBias   float64  `json:"vBias"`   // DC bias for MLCC derating
	Package string   `json:"package"` // 0603, 1210, radial...
	InBOM   bool     `json:"inBom"`

	// Requirements used for part search
	ReqV    float64 `json:"reqV"`
	ReqVAC  bool    `json:"reqVac"`
	ReqI    float64 `json:"reqI"`
	ReqIsat float64 `json:"reqIsat"`
	Class   string  `json:"class"` // X2 / Y1/Y2 / X7R ...

	Part *PartInfo `json:"part,omitempty"`
}

// Stage is used by the UI to draw the ladder schematic.
type Stage struct {
	Type string   `json:"type"` // shunt | shunt_series | series_top | series_bot | cmc | y | lisn | conv
	Refs []string `json:"refs"`
}

type Circuit struct {
	Comps       []Comp   `json:"comps"`
	Layout      []Stage  `json:"layout"`
	InTop       string   `json:"inTop"`
	InBot       string   `json:"inBot"`
	OutTop      string   `json:"outTop"`
	OutBot      string   `json:"outBot"`
	Description string   `json:"description"`
	Notes       []string `json:"notes"`
}

func (c *Circuit) Find(ref string) *Comp {
	for i := range c.Comps {
		if c.Comps[i].Ref == ref {
			return &c.Comps[i]
		}
	}
	return nil
}

// EffC returns the effective capacitance of a capacitor comp (per piece),
// including an approximate class-II MLCC DC-bias derating.
func (c Comp) EffC() float64 {
	if c.Kind != "cap" {
		return c.Value
	}
	return c.Value * dcBiasFactor(c)
}

func dcBiasFactor(c Comp) float64 {
	if c.Sub != "mlcc" || c.VRated <= 0 || c.VBias <= 0 || c.Class == "C0G" {
		return 1
	}
	x := c.VBias / c.VRated
	if x > 1 {
		x = 1
	}
	// Empirical fit for high-CV X7R/X5R parts: -15% @ 0.25Vr, -35% @ 0.5Vr, -70% @ Vr.
	f := 1 - 0.72*math.Pow(x, 1.3)
	if c.Value < 0.5e-6 {
		// small-value MLCCs derate much less
		f = 1 - 0.3*math.Pow(x, 1.5)
	}
	return math.Max(f, 0.2)
}
