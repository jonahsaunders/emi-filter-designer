package engine

import "math"

func sinc(x float64) float64 {
	if math.Abs(x) < 1e-12 {
		return 1
	}
	return math.Sin(x) / x
}

// trapezoid harmonic peak amplitude: pulse height A, duty D, rise = fall = tr.
func trapHarm(A, D, tr, T float64, n int) float64 {
	nf := float64(n)
	return 2 * A * D * math.Abs(sinc(math.Pi*nf*D)) * math.Abs(sinc(math.Pi*nf*tr/T))
}

// triangle (inductor ripple) harmonic peak amplitude: p-p ripple dI, rising
// for D*T.
func triHarm(dI, D float64, n int) float64 {
	nf := float64(n)
	if D <= 0 || D >= 1 {
		return 0
	}
	return dI / (math.Pi * math.Pi * nf * nf * D * (1 - D)) * math.Abs(math.Sin(math.Pi*nf*D))
}

// OpPoint is one steady-state operating point used for the noise model.
type OpPoint struct {
	Label string  `json:"label"`
	Vin   float64 `json:"vin"`  // DC input (or rectified peak for AC)
	Iin   float64 `json:"iin"`  // average input current
	D     float64 `json:"d"`    // duty cycle of the switch
	Vsw   float64 `json:"vsw"`  // switch-node voltage swing (CM source)
	Ipk   float64 `json:"ipk"`  // pulse height of input current (pulsed topologies)
	Ripp  float64 `json:"ripp"` // p-p ripple (continuous-input topologies)
	Shape string  `json:"shape"`
}

func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }

func opPointFor(p Params, label string, vin float64) OpPoint {
	eff := clamp(p.Eff, 0.3, 1)
	pin := p.Pout / eff
	op := OpPoint{Label: label, Vin: vin, Iin: pin / vin}
	switch p.Topology {
	case "boost", "pfc":
		vo := p.Vout
		if p.Mode == "ac" && vo < vin*1.05 {
			vo = math.Max(400, vin*1.1)
		}
		if vo < vin*1.05 {
			vo = vin * 1.05
		}
		op.D = clamp(1-vin/vo, 0.02, 0.95)
		iAvg := op.Iin
		if p.Mode == "ac" {
			// PFC: at the peak of the line the inductor current is 2x the
			// cycle-average input current (sinusoidal current, PF~1)
			iAvg = 2 * op.Iin
		}
		op.Ripp = clamp(p.RippleRatio, 0.05, 2) * iAvg
		op.Vsw = vo
		op.Shape = "triangle"
	case "buck":
		op.D = clamp(p.Vout/(vin*math.Sqrt(eff)), 0.03, 0.97)
		op.Ipk = op.Iin / op.D
		op.Vsw = vin
		op.Shape = "trapezoid"
	case "buckboost":
		op.D = clamp(p.Vout/(p.Vout+vin), 0.03, 0.97)
		op.Ipk = op.Iin / op.D
		op.Vsw = vin + p.Vout
		op.Shape = "trapezoid"
	default: // flyback
		vr := p.Vrefl
		if vr <= 0 {
			vr = vin * 0.5
		}
		op.D = clamp(vr/(vr+vin), 0.03, 0.9)
		op.Ipk = op.Iin / op.D
		op.Vsw = vin + vr
		op.Shape = "trapezoid"
	}
	return op
}

// OpPoints returns the operating points evaluated (min / nom / max input).
func OpPoints(p Params) []OpPoint {
	if p.Mode == "ac" {
		ops := []OpPoint{
			opPointFor(p, "Vac min", math.Sqrt2*p.VacMin),
			opPointFor(p, "Vac max", math.Sqrt2*p.VacMax),
		}
		return ops
	}
	return []OpPoint{
		opPointFor(p, "Vin min", p.VinMin),
		opPointFor(p, "Vin nom", p.VinNom),
		opPointFor(p, "Vin max", p.VinMax),
	}
}

// NoiseSpectrum returns harmonic frequencies and the worst-case (over the
// operating points) DM current and CM voltage source amplitudes (peak).
func NoiseSpectrum(p Params, fmin, fmax float64) (f, idm, vcm []float64) {
	T := 1 / p.Fsw
	ops := OpPoints(p)
	tr := math.Max(p.RiseTime, 1e-10)
	for n := 1; float64(n)*p.Fsw <= fmax; n++ {
		fn := float64(n) * p.Fsw
		if fn < fmin {
			continue
		}
		var di, dv float64
		for _, op := range ops {
			var a float64
			if op.Shape == "triangle" {
				a = triHarm(op.Ripp, op.D, n)
				// commutation edge adds a small trapezoidal component
				a = math.Max(a, trapHarm(op.Ripp*0.05, op.D, tr, T, n))
			} else {
				a = trapHarm(op.Ipk, op.D, tr, T, n)
			}
			di = math.Max(di, a)
			dv = math.Max(dv, trapHarm(op.Vsw, op.D, tr, T, n))
		}
		f = append(f, fn)
		idm = append(idm, di)
		vcm = append(vcm, dv)
	}
	return
}

// Converter input impedance magnitude used for the Middlebrook check
// (closed-loop constant-power load: |Zin| = Vmin^2 / Pin).
func ConverterZin(p Params) float64 {
	pin := p.Pout / clamp(p.Eff, 0.3, 1)
	v := p.VinMin
	if p.Mode == "ac" {
		v = p.VacMin
		if p.Topology == "flyback" {
			v = math.Sqrt2 * p.VacMin * 0.85
		}
	}
	return v * v / pin
}

// InputCurrentMax returns worst-case continuous current through the filter
// (DC: at Vin min; AC: rms line current at Vac min).
func InputCurrentMax(p Params) float64 {
	pin := p.Pout / clamp(p.Eff, 0.3, 1)
	if p.Mode == "ac" {
		pf := clamp(p.PF, 0.3, 1)
		return pin / (pf * p.VacMin)
	}
	return pin / p.VinMin
}

func dbuv(vpk float64) float64 {
	vr := vpk / math.Sqrt2
	if vr < 1e-12 {
		vr = 1e-12
	}
	return 20 * math.Log10(vr/1e-6)
}
