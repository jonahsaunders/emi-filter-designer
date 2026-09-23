package parts

import (
	"math"
	"testing"

	"emifilter/engine"
)

func TestParseQty(t *testing.T) {
	cases := []struct {
		s, u string
		v    float64
	}{
		{"Multilayer Ceramic Capacitors MLCC - SMD/SMT 50V 10uF X7R 1210 10%", "F", 10e-6},
		{"Multilayer Ceramic Capacitors MLCC - SMD/SMT 50V 10uF X7R 1210 10%", "V", 50},
		{"Fixed Inductors 10uH 20% 7.6A 29.8mOhm", "A", 7.6},
		{"Fixed Inductors 10uH 20% 7.6A 29.8mOhm", "Ohm", 29.8e-3},
		{"10 µH", "H", 10e-6},
		{"29.8mOhm Max", "Ohm", 29.8e-3},
		{"2200 pF", "F", 2.2e-9},
		{"14MHz", "Hz", 14e6},
		{"305V", "V", 305},
	}
	for _, c := range cases {
		v, ok := ParseQty(c.s, c.u)
		if !ok || v < c.v*0.999 || v > c.v*1.001 {
			t.Errorf("ParseQty(%q,%s) = %g, want %g", c.s, c.u, v, c.v)
		}
	}
}

func TestParseMouser(t *testing.T) {
	raw := []byte(`{"Errors":[],"SearchResults":{"NumberOfResult":1,"Parts":[{"Availability":"12,345 In Stock","AvailabilityInStock":"12345",
	"DataSheetUrl":"https://x/ds.pdf","Description":"Fixed Inductors 10uH 20% 7.6A 29.8mOhm","Manufacturer":"Coilcraft",
	"ManufacturerPartNumber":"XAL6060-103MEC","MouserPartNumber":"994-XAL6060-103MEC","ProductDetailUrl":"https://www.mouser.com/ProductDetail/x",
	"PriceBreaks":[{"Quantity":1,"Price":"$3.12","Currency":"USD"},{"Quantity":10,"Price":"$2.50","Currency":"USD"}],
	"ProductAttributes":[{"AttributeName":"Packaging","AttributeValue":"Reel"}]}]}}`)
	cs, err := parseMouser(raw)
	if err != nil || len(cs) != 1 {
		t.Fatal(err, len(cs))
	}
	c := cs[0]
	if c.Stock != 12345 || c.UnitPrice != 3.12 || math.Abs(c.Value-10e-6) > 1e-12 || c.IRated != 7.6 || c.DCR < 0.0297 {
		t.Fatalf("%+v", c)
	}
}

func TestParseDigikey(t *testing.T) {
	raw := []byte(`{"Products":[{"Description":{"ProductDescription":"CAP CER 10UF 50V X7R 1210","DetailedDescription":"10 µF ±10% 50V Ceramic Capacitor X7R 1210 (3225 Metric)"},
	"Manufacturer":{"Id":1,"Name":"Murata Electronics"},"ManufacturerProductNumber":"GRM32ER71H106KA12L","UnitPrice":0.62,
	"ProductUrl":"https://www.digikey.com/en/products/detail/x","DatasheetUrl":"https://x.pdf","QuantityAvailable":50000,
	"Parameters":[{"ParameterText":"Capacitance","ValueText":"10 µF"},{"ParameterText":"Voltage - Rated","ValueText":"50V"},{"ParameterText":"Package / Case","ValueText":"1210 (3225 Metric)"}],
	"ProductVariations":[{"DigiKeyProductNumber":"490-1-ND","StandardPricing":[{"BreakQuantity":1,"UnitPrice":0.62}]}]}],
	"SearchLocaleUsed":{"Site":"US","Language":"en","Currency":"USD"}}`)
	cs, err := parseDigikey(raw, "USD")
	if err != nil || len(cs) != 1 {
		t.Fatal(err)
	}
	c := cs[0]
	if math.Abs(c.Value-10e-6) > 1e-12 || c.VRated != 50 || c.Package != "1210" || c.Stock != 50000 || c.DistPN != "490-1-ND" {
		t.Fatalf("%+v", c)
	}
	r := Rank(engine.Comp{Kind: "cap", Value: 10e-6, ReqV: 50}, cs)
	if r[0].Match != "ok" {
		t.Fatalf("match %s", r[0].Match)
	}
}
