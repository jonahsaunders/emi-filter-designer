package engine

import (
	"math"
)

// expandComp adds the parasitic model of a component to the netlist.
func expandComp(nl *Netlist, c Comp) {
	q := float64(c.Qty)
	if q < 1 {
		q = 1
	}
	pre := c.Ref
	switch c.Kind {
	case "cap":
		if len(c.Nodes) < 2 {
			return
		}
		a, b := c.Nodes[0], c.Nodes[1]
		esr := math.Max(c.ESR, 1e-4) / q
		esl := c.ESL / q
		x := pre + ".x"
		nl.R(pre+".esr", a, x, esr)
		y := x
		if esl > 0 {
			y = pre + ".y"
			nl.L(pre+".esl", x, y, esl)
		}
		nl.C(pre+".c", y, b, c.EffC()*q)
	case "res":
		if len(c.Nodes) < 2 {
			return
		}
		r := c.Value / q
		if c.ESL > 0 {
			x := pre + ".x"
			nl.R(pre+".r", c.Nodes[0], x, r)
			nl.L(pre+".esl", x, c.Nodes[1], c.ESL/q)
		} else {
			nl.R(pre+".r", c.Nodes[0], c.Nodes[1], r)
		}
	case "ind":
		if len(c.Nodes) < 2 {
			return
		}
		a, b := c.Nodes[0], c.Nodes[1]
		x := pre + ".x"
		nl.R(pre+".dcr", a, x, math.Max(c.DCR, 1e-5))
		nl.L(pre+".l", x, b, c.Value)
		if c.SRF > 0 && c.Value > 0 {
			w := 2 * math.Pi * c.SRF
			nl.C(pre+".cp", x, b, 1/(w*w*c.Value))
			q := c.Qsrf
			if q <= 0 {
				q = 5
			}
			nl.R(pre+".rp", x, b, q*w*c.Value)
		}
	case "cmc":
		if len(c.Nodes) < 4 {
			return
		}
		L := c.Value
		leak := c.Leak
		if leak <= 0 {
			leak = 0.01 * L
		}
		k := 1 - leak/(2*L)
		if k < 0.5 {
			k = 0.5
		}
		for i, w := range []string{"a", "b"} {
			n1, n2 := c.Nodes[2*i], c.Nodes[2*i+1]
			x := pre + "." + w + "x"
			nl.R(pre+".dcr"+w, n1, x, math.Max(c.DCR, 1e-5))
			nl.L(pre+".l"+w, x, n2, L)
			if c.SRF > 0 {
				om := 2 * math.Pi * c.SRF
				nl.C(pre+".cp"+w, x, n2, 1/(om*om*L))
				qs := c.Qsrf
				if qs <= 0 {
					qs = 1.5
				}
				nl.R(pre+".rp"+w, x, n2, qs*om*L)
			}
		}
		_ = nl.K(pre+".k", pre+".la", pre+".lb", k)
	}
}

// addLISN adds one artificial network (per line).  The receiver (50 ohm)
// voltage is at node LM<id>.
func addLISN(nl *Netlist, typ, node, id string) {
	ls := "LS" + id
	lm := "LM" + id
	if typ == "5uH" {
		// CISPR 25 AN: 5 uH, 1 uF supply-side, 0.1 uF + 1 kohm // 50 ohm
		nl.L("LLISN"+id, node, ls, 5e-6)
		nl.C("CLISNS"+id, ls, "0", 1e-6)
		nl.R("RLISNS"+id, ls, "0", 1e3)
		// battery / bench supply + cabling behind the network
		nl.R("RBAT"+id, ls, "BAT"+id, 0.02)
		nl.L("LBAT"+id, "BAT"+id, "0", 0.5e-6)
	} else {
		// CISPR 16-1-2 50 uH + 5 ohm V-network: Z = 50 // (50 uH + 5 ohm)
		nl.L("LLISN"+id, node, ls, 50e-6)
		nl.R("RLISNS"+id, ls, "0", 5)
	}
	nl.C("CLISN"+id, node, lm, 0.1e-6)
	nl.R("RLISNK"+id, lm, "0", 1e3)
	nl.R("RLISNM"+id, lm, "0", 50)
}

type netOpts struct {
	filter bool // include BOM (filter) components; otherwise input shorted to output
	conv   bool // converter model (input cap + noise sources)
	lisn   bool
	test   bool // 1 A test current into the output port (Zout)
}

func singleLISN(p Params) bool { return p.Mode == "dc" && p.ReturnGrounded }

func buildNet(p Params, c *Circuit, o netOpts) *Netlist {
	nl := NewNetlist()
	std := GetStandard(p.Standard)
	if o.lisn {
		addLISN(nl, std.LISN, c.InTop, "1")
		if singleLISN(p) {
			nl.R("RGNDRET", c.InBot, "0", 1e-3)
		} else {
			addLISN(nl, std.LISN, c.InBot, "2")
		}
	}
	if o.filter {
		for _, cp := range c.Comps {
			if cp.InBOM {
				expandComp(nl, cp)
			}
		}
	} else {
		nl.R("RSHT", c.InTop, c.OutTop, 1e-6)
		nl.R("RSHB", c.InBot, c.OutBot, 1e-6)
	}
	if o.conv {
		for _, cp := range c.Comps {
			if !cp.InBOM {
				expandComp(nl, cp)
			}
		}
		nl.I("IDM", c.OutTop, c.OutBot, "dm")
		nl.V("VCM", "SWN", c.OutBot, "cm")
		nl.C("CPAR", "SWN", "0", math.Max(p.Cp, 1e-15))
	}
	if o.test {
		nl.I("ITEST", c.OutBot, c.OutTop, "test")
	}
	// make sure every node has at least a weak path (gmin is also added)
	return nl
}

// buildIL builds the CISPR 17-style 50 ohm / 50 ohm insertion-loss fixture.
// mode "dm": symmetric 0.5 V sources, 25 ohm per line, 25 ohm loads per line.
// mode "cm": input lines driven together via 100 ohm each (50 ohm total),
// output lines loaded with 100 ohm each to ground.
func buildIL(c *Circuit, mode string, filter bool) *Netlist {
	nl := NewNetlist()
	if mode == "dm" {
		nl.V("VSP", "SP", "0", "p")
		nl.V("VSN", "SN", "0", "n")
		nl.R("RSP", "SP", c.InTop, 25)
		nl.R("RSN", "SN", c.InBot, 25)
		nl.R("RLP", c.OutTop, "0", 25)
		nl.R("RLN", c.OutBot, "0", 25)
	} else {
		nl.V("VS", "S", "0", "p")
		nl.R("RSP", "S", c.InTop, 100)
		nl.R("RSN", "S", c.InBot, 100)
		nl.R("RLP", c.OutTop, "0", 100)
		nl.R("RLN", c.OutBot, "0", 100)
	}
	if filter {
		for _, cp := range c.Comps {
			if cp.InBOM {
				expandComp(nl, cp)
			}
		}
	} else {
		nl.R("RSHT", c.InTop, c.OutTop, 1e-6)
		nl.R("RSHB", c.InBot, c.OutBot, 1e-6)
	}
	return nl
}
