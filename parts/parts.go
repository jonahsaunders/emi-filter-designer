// Package parts implements live component search through the Mouser Search
// API (v1) and the DigiKey Product Information API (v4), and normalises the
// results into a common Candidate structure.
package parts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"emifilter/engine"
)

type Settings struct {
	MouserKey       string `json:"mouserKey"`
	DigikeyClientID string `json:"digikeyClientId"`
	DigikeySecret   string `json:"digikeySecret"`
	DigikeySite     string `json:"digikeySite"`     // US, DE, UK ...
	DigikeyCurrency string `json:"digikeyCurrency"` // USD, EUR ...
	InStockOnly     bool   `json:"inStockOnly"`
}

// Candidate is a normalised search result.
type Candidate struct {
	Source       string            `json:"source"` // library | mouser | digikey
	MPN          string            `json:"mpn"`
	Manufacturer string            `json:"manufacturer"`
	Description  string            `json:"description"`
	DistPN       string            `json:"distPn"`
	UnitPrice    float64           `json:"unitPrice"`
	Currency     string            `json:"currency"`
	Stock        int               `json:"stock"`
	ProductURL   string            `json:"productUrl"`
	MouserURL    string            `json:"mouserUrl"`
	DigikeyURL   string            `json:"digikeyUrl"`
	DatasheetURL string            `json:"datasheetUrl"`
	Value        float64           `json:"value"`
	VRated       float64           `json:"vRated"`
	IRated       float64           `json:"iRated"`
	ISat         float64           `json:"iSat"`
	DCR          float64           `json:"dcr"`
	SRF          float64           `json:"srf"`
	ESR          float64           `json:"esr"`
	Package      string            `json:"package"`
	Match        string            `json:"match"` // ok | value | rating | unknown
	Score        float64           `json:"score"`
	Params       map[string]string `json:"params,omitempty"`
}

var httpClient = &http.Client{Timeout: 25 * time.Second}

// ---------------------------------------------------------------------------
// Value parsing

var siPrefix = map[string]float64{"p": 1e-12, "n": 1e-9, "u": 1e-6, "µ": 1e-6, "μ": 1e-6, "m": 1e-3, "": 1, "k": 1e3, "K": 1e3, "M": 1e6, "G": 1e9}

var reQty = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*([pnu\x{00b5}\x{03bc}mkKMG]?)\s*(F|H|Ohm|\x{03a9}|V|A|Hz)\b`)

// ParseQty parses the first quantity with the given unit ("F","H","Ohm","V","A","Hz").
func ParseQty(s, unit string) (float64, bool) {
	s = strings.ReplaceAll(s, "Ω", "Ω")
	for _, m := range reQty.FindAllStringSubmatch(s, -1) {
		u := m[3]
		if strings.EqualFold(u, "ohm") || u == "Ω" {
			u = "Ohm"
		}
		if !strings.EqualFold(u, unit) {
			continue
		}
		v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
		if err != nil {
			continue
		}
		p := m[2]
		if p == "M" && (unit == "F" || unit == "H") {
			p = "m" // "MF"/"MH" never mean mega
		}
		mult, ok := siPrefix[p]
		if !ok {
			mult = 1
		}
		return v * mult, true
	}
	return 0, false
}

func parsePrice(s string) float64 {
	s = strings.TrimSpace(s)
	// keep digits, separators
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' {
			b.WriteRune(r)
		}
	}
	t := b.String()
	if strings.Count(t, ",") == 1 && !strings.Contains(t, ".") {
		t = strings.Replace(t, ",", ".", 1)
	} else {
		t = strings.ReplaceAll(t, ",", "")
	}
	v, _ := strconv.ParseFloat(t, 64)
	return v
}

func parseInt(s string) int {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	v, _ := strconv.Atoi(b.String())
	return v
}

// ---------------------------------------------------------------------------
// Mouser

type mouserResp struct {
	Errors []struct {
		Message string `json:"Message"`
	} `json:"Errors"`
	SearchResults *struct {
		NumberOfResult int `json:"NumberOfResult"`
		Parts          []struct {
			Availability           string `json:"Availability"`
			AvailabilityInStock    string `json:"AvailabilityInStock"`
			DataSheetURL           string `json:"DataSheetUrl"`
			Description            string `json:"Description"`
			Manufacturer           string `json:"Manufacturer"`
			ManufacturerPartNumber string `json:"ManufacturerPartNumber"`
			MouserPartNumber       string `json:"MouserPartNumber"`
			ProductDetailURL       string `json:"ProductDetailUrl"`
			Category               string `json:"Category"`
			PriceBreaks            []struct {
				Quantity int    `json:"Quantity"`
				Price    string `json:"Price"`
				Currency string `json:"Currency"`
			} `json:"PriceBreaks"`
			ProductAttributes []struct {
				AttributeName  string `json:"AttributeName"`
				AttributeValue string `json:"AttributeValue"`
			} `json:"ProductAttributes"`
		} `json:"Parts"`
	} `json:"SearchResults"`
}

func MouserSearch(s Settings, keyword string, limit int) ([]Candidate, error) {
	if s.MouserKey == "" {
		return nil, errors.New("no Mouser API key configured")
	}
	opt := "None"
	if s.InStockOnly {
		opt = "InStock"
	}
	body := map[string]any{"SearchByKeywordRequest": map[string]any{
		"keyword": keyword, "records": limit, "startingRecord": 0, "searchOptions": opt, "searchWithYourSignUpLanguage": "false"}}
	b, _ := json.Marshal(body)
	u := "https://api.mouser.com/api/v1/search/keyword?apiKey=" + url.QueryEscape(s.MouserKey)
	req, _ := http.NewRequest("POST", u, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Mouser: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Mouser: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	return parseMouser(raw)
}

func parseMouser(raw []byte) ([]Candidate, error) {
	var r mouserResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("Mouser: bad response: %w", err)
	}
	if len(r.Errors) > 0 && r.Errors[0].Message != "" {
		return nil, fmt.Errorf("Mouser: %s", r.Errors[0].Message)
	}
	var out []Candidate
	if r.SearchResults == nil {
		return out, nil
	}
	for _, p := range r.SearchResults.Parts {
		if p.ManufacturerPartNumber == "" {
			continue
		}
		c := Candidate{Source: "mouser", MPN: p.ManufacturerPartNumber, Manufacturer: p.Manufacturer, Description: p.Description,
			DistPN: p.MouserPartNumber, ProductURL: p.ProductDetailURL, DatasheetURL: p.DataSheetURL, Params: map[string]string{},
			MouserURL: firstNonEmpty(p.ProductDetailURL, engine.MouserURL(p.ManufacturerPartNumber)), DigikeyURL: engine.DigikeyURL(p.ManufacturerPartNumber)}
		c.Stock = parseInt(p.AvailabilityInStock)
		if c.Stock == 0 {
			if f := strings.Fields(p.Availability); len(f) > 0 {
				c.Stock = parseInt(f[0])
			}
		}
		for _, pb := range p.PriceBreaks {
			if pb.Quantity <= 1 || c.UnitPrice == 0 {
				c.UnitPrice = parsePrice(pb.Price)
				c.Currency = pb.Currency
			}
		}
		text := p.Description
		for _, a := range p.ProductAttributes {
			c.Params[a.AttributeName] = a.AttributeValue
			text += " " + a.AttributeName + ": " + a.AttributeValue
		}
		fillFromText(&c, text)
		out = append(out, c)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// DigiKey (OAuth2 client-credentials, Product Information v4)

var dkTok struct {
	sync.Mutex
	id, token string
	exp       time.Time
}

func digikeyToken(s Settings) (string, error) {
	dkTok.Lock()
	defer dkTok.Unlock()
	if dkTok.token != "" && dkTok.id == s.DigikeyClientID && time.Now().Before(dkTok.exp) {
		return dkTok.token, nil
	}
	form := url.Values{"client_id": {s.DigikeyClientID}, "client_secret": {s.DigikeySecret}, "grant_type": {"client_credentials"}}
	resp, err := httpClient.PostForm("https://api.digikey.com/v1/oauth2/token", form)
	if err != nil {
		return "", fmt.Errorf("DigiKey auth: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("DigiKey auth: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &t); err != nil || t.AccessToken == "" {
		return "", fmt.Errorf("DigiKey auth: unexpected response")
	}
	dkTok.id, dkTok.token = s.DigikeyClientID, t.AccessToken
	dkTok.exp = time.Now().Add(time.Duration(max(t.ExpiresIn-60, 60)) * time.Second)
	return t.AccessToken, nil
}

type dkResp struct {
	Products []struct {
		Description struct {
			ProductDescription  string `json:"ProductDescription"`
			DetailedDescription string `json:"DetailedDescription"`
		} `json:"Description"`
		Manufacturer struct {
			Name string `json:"Name"`
		} `json:"Manufacturer"`
		ManufacturerProductNumber string  `json:"ManufacturerProductNumber"`
		UnitPrice                 float64 `json:"UnitPrice"`
		ProductURL                string  `json:"ProductUrl"`
		DatasheetURL              string  `json:"DatasheetUrl"`
		QuantityAvailable         float64 `json:"QuantityAvailable"`
		Parameters                []struct {
			ParameterText string `json:"ParameterText"`
			ValueText     string `json:"ValueText"`
		} `json:"Parameters"`
		ProductVariations []struct {
			DigiKeyProductNumber string `json:"DigiKeyProductNumber"`
			StandardPricing      []struct {
				BreakQuantity float64 `json:"BreakQuantity"`
				UnitPrice     float64 `json:"UnitPrice"`
			} `json:"StandardPricing"`
		} `json:"ProductVariations"`
	} `json:"Products"`
	SearchLocaleUsed *struct {
		Currency string `json:"Currency"`
	} `json:"SearchLocaleUsed"`
}

func DigikeySearch(s Settings, keyword string, limit int) ([]Candidate, error) {
	if s.DigikeyClientID == "" || s.DigikeySecret == "" {
		return nil, errors.New("no DigiKey client ID / secret configured")
	}
	tok, err := digikeyToken(s)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"Keywords": keyword, "Limit": limit, "Offset": 0}
	if s.InStockOnly {
		body["FilterOptionsRequest"] = map[string]any{"SearchOptions": []string{"InStock"}}
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "https://api.digikey.com/products/v4/search/keyword", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-DIGIKEY-Client-Id", s.DigikeyClientID)
	site, cur := s.DigikeySite, s.DigikeyCurrency
	if site == "" {
		site = "US"
	}
	if cur == "" {
		cur = "USD"
	}
	req.Header.Set("X-DIGIKEY-Locale-Site", site)
	req.Header.Set("X-DIGIKEY-Locale-Language", "en")
	req.Header.Set("X-DIGIKEY-Locale-Currency", cur)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DigiKey: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 401 {
		dkTok.Lock()
		dkTok.token = ""
		dkTok.Unlock()
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("DigiKey: HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	return parseDigikey(raw, cur)
}

func parseDigikey(raw []byte, cur string) ([]Candidate, error) {
	var r dkResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("DigiKey: bad response: %w", err)
	}
	if r.SearchLocaleUsed != nil && r.SearchLocaleUsed.Currency != "" {
		cur = r.SearchLocaleUsed.Currency
	}
	var out []Candidate
	for _, p := range r.Products {
		c := Candidate{Source: "digikey", MPN: p.ManufacturerProductNumber, Manufacturer: p.Manufacturer.Name,
			Description: firstNonEmpty(p.Description.DetailedDescription, p.Description.ProductDescription),
			UnitPrice:   p.UnitPrice, Currency: cur, Stock: int(p.QuantityAvailable), ProductURL: p.ProductURL,
			DatasheetURL: p.DatasheetURL, Params: map[string]string{},
			DigikeyURL: firstNonEmpty(p.ProductURL, engine.DigikeyURL(p.ManufacturerProductNumber)), MouserURL: engine.MouserURL(p.ManufacturerProductNumber)}
		if len(p.ProductVariations) > 0 {
			c.DistPN = p.ProductVariations[0].DigiKeyProductNumber
			if c.UnitPrice == 0 {
				for _, sp := range p.ProductVariations[0].StandardPricing {
					if sp.BreakQuantity <= 1 || c.UnitPrice == 0 {
						c.UnitPrice = sp.UnitPrice
					}
				}
			}
		}
		text := c.Description + " " + p.Description.ProductDescription
		for _, pm := range p.Parameters {
			c.Params[pm.ParameterText] = pm.ValueText
			applyParam(&c, pm.ParameterText, pm.ValueText)
		}
		fillFromText(&c, text)
		out = append(out, c)
	}
	return out, nil
}

func applyParam(c *Candidate, name, val string) {
	n := strings.ToLower(name)
	switch {
	case n == "capacitance":
		if v, ok := ParseQty(val, "F"); ok {
			c.Value = v
		}
	case n == "inductance":
		if v, ok := ParseQty(val, "H"); ok {
			c.Value = v
		}
	case n == "resistance":
		if v, ok := ParseQty(val, "Ohm"); ok {
			c.Value = v
		}
	case strings.HasPrefix(n, "voltage - rated") || strings.HasPrefix(n, "voltage rating") || n == "voltage - ac" || n == "voltage rating - ac" || n == "voltage rating - dc":
		if v, ok := ParseQty(val, "V"); ok && v > c.VRated {
			c.VRated = v
		}
	case strings.HasPrefix(n, "current rating") || strings.HasPrefix(n, "current - rated"):
		if v, ok := ParseQty(val, "A"); ok {
			c.IRated = v
		}
	case strings.HasPrefix(n, "current - saturation"):
		if v, ok := ParseQty(val, "A"); ok {
			c.ISat = v
		}
	case strings.HasPrefix(n, "dc resistance"):
		if v, ok := ParseQty(val, "Ohm"); ok {
			c.DCR = v
		}
	case strings.HasPrefix(n, "frequency - self resonant"):
		if v, ok := ParseQty(val, "Hz"); ok {
			c.SRF = v
		}
	case strings.HasPrefix(n, "esr"):
		if v, ok := ParseQty(val, "Ohm"); ok {
			c.ESR = v
		}
	case n == "package / case" || n == "case code - in" || n == "size / dimension":
		if c.Package == "" {
			c.Package = strings.SplitN(val, " ", 2)[0]
		}
	}
}

var rePkg = regexp.MustCompile(`\b(0201|0402|0603|0805|1206|1210|1812|2220)\b`)

// fillFromText extracts missing values from a free-text description
// (Mouser descriptions such as "Multilayer Ceramic Capacitors MLCC - SMD/SMT
// 50V 10uF X7R 1210 10%" or "Fixed Inductors 10uH 20% 7.6A 29.8mOhm").
func fillFromText(c *Candidate, text string) {
	if c.Value == 0 {
		for _, u := range []string{"F", "H"} {
			if v, ok := ParseQty(text, u); ok {
				c.Value = v
				break
			}
		}
	}
	if c.VRated == 0 {
		if v, ok := ParseQty(text, "V"); ok {
			c.VRated = v
		}
	}
	if c.IRated == 0 {
		if v, ok := ParseQty(text, "A"); ok {
			c.IRated = v
		}
	}
	if c.DCR == 0 {
		if v, ok := ParseQty(text, "Ohm"); ok && v < 50 {
			c.DCR = v
		}
	}
	if c.Package == "" {
		if m := rePkg.FindString(text); m != "" {
			c.Package = m
		}
	}
}

// ---------------------------------------------------------------------------
// Ranking

func Rank(comp engine.Comp, cs []Candidate) []Candidate {
	for i := range cs {
		c := &cs[i]
		c.Match = "ok"
		s := 0.0
		if c.Value > 0 && comp.Value > 0 {
			r := math.Abs(math.Log(c.Value / comp.Value))
			s += r * 10
			if r > math.Log(1.25) {
				c.Match = "value"
			}
		} else {
			c.Match = "unknown"
			s += 8
		}
		switch comp.Kind {
		case "cap":
			if comp.ReqV > 0 && c.VRated > 0 && c.VRated < comp.ReqV && !comp.ReqVAC {
				c.Match, s = "rating", s+20
			}
		case "ind", "cmc":
			if comp.ReqI > 0 && c.IRated > 0 && c.IRated < comp.ReqI {
				c.Match, s = "rating", s+20
			}
			if comp.ReqIsat > 0 && c.ISat > 0 && c.ISat < comp.ReqIsat {
				c.Match, s = "rating", s+20
			}
			if c.DCR > 0 {
				s += c.DCR * 20
			}
		}
		if c.Stock <= 0 && c.Source != "library" {
			s += 5
		}
		if c.UnitPrice > 0 {
			s += math.Log10(c.UnitPrice+0.01) * 0.8
		}
		c.Score = s
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Score < cs[j].Score })
	return cs
}

// FromLibrary converts library candidates.
func FromLibrary(ls []engine.LibPart) []Candidate {
	var out []Candidate
	for _, p := range ls {
		out = append(out, Candidate{Source: "library", MPN: p.MPN, Manufacturer: p.Mfr, Description: p.Desc, Value: p.Value,
			VRated: p.VRated, IRated: p.IRms, ISat: p.ISat, DCR: p.DCR, SRF: p.SRF, ESR: p.ESR, Package: p.Pkg,
			MouserURL: engine.MouserURL(p.MPN), DigikeyURL: engine.DigikeyURL(p.MPN)})
	}
	return out
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// Apply copies the relevant data of a chosen candidate into a filter
// component (value, parasitics where known, and purchasing information).
func Apply(c engine.Comp, k Candidate) engine.Comp {
	if k.Source == "library" {
		if lp, ok := engine.LibByMPN(k.MPN); ok {
			lp.ApplyTo(&c)
			return c
		}
	}
	verified := false
	if k.Value > 0 && c.Value > 0 {
		if r := k.Value / c.Value; r > 0.2 && r < 5 {
			c.Value = k.Value
			verified = true
		}
	}
	if k.VRated > 0 {
		c.VRated = k.VRated
	}
	if k.DCR > 0 {
		c.DCR = k.DCR
	}
	if k.SRF > 0 {
		c.SRF = k.SRF
	}
	if k.ESR > 0 {
		c.ESR = k.ESR
	}
	if k.Package != "" {
		c.Package = k.Package
		if c.Sub == "mlcc" {
			if esl, ok := engine.MLCCMountedESL(k.Package); ok {
				c.ESL = esl
			}
		}
	}
	c.Part = &engine.PartInfo{MPN: k.MPN, Manufacturer: k.Manufacturer, Description: k.Description, Source: k.Source,
		UnitPrice: k.UnitPrice, Currency: k.Currency, Stock: k.Stock, MouserURL: k.MouserURL, DigikeyURL: k.DigikeyURL,
		DatasheetURL: k.DatasheetURL, DistPN: k.DistPN, Verified: verified}
	return c
}

// ParseMouser parses a raw Mouser keyword-search response (used by the
// browser build, where the HTTP request is made by JavaScript).
func ParseMouser(raw []byte) ([]Candidate, error) { return parseMouser(raw) }

// ParseDigikey parses a raw DigiKey v4 keyword-search response.
func ParseDigikey(raw []byte, currency string) ([]Candidate, error) {
	return parseDigikey(raw, currency)
}
