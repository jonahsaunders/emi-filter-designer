// EMI Filter Designer - desktop application.
//
// A local HTTP server (127.0.0.1, random port) serves the embedded UI and the
// design / simulation / part-search API.  On start-up the UI is opened in a
// chromeless Microsoft Edge (or Chrome) "app" window; if neither is found the
// default browser is used.  The process exits shortly after the UI window is
// closed.
package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"emifilter/engine"
	"emifilter/parts"
)

const version = "1.0.0"

//go:embed web
var webFS embed.FS

var (
	cfgDir     string
	settingsMu sync.Mutex
	settings   parts.Settings
	lastPing   = time.Now()
	pingMu     sync.Mutex
	quitAt     time.Time
)

func configDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = "."
	}
	d = filepath.Join(d, "EMI Filter Designer")
	_ = os.MkdirAll(d, 0o755)
	return d
}

func loadSettings() {
	b, err := os.ReadFile(filepath.Join(cfgDir, "settings.json"))
	if err == nil {
		_ = json.Unmarshal(b, &settings)
	}
	if settings.DigikeySite == "" {
		settings.DigikeySite = "US"
	}
	if settings.DigikeyCurrency == "" {
		settings.DigikeyCurrency = "USD"
	}
}

func saveSettings() error {
	b, _ := json.MarshalIndent(settings, "", "  ")
	return os.WriteFile(filepath.Join(cfgDir, "settings.json"), b, 0o600)
}

// ---------------------------------------------------------------------------
// HTTP helpers

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		log.Println("encode:", err)
	}
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func recoverer(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if e := recover(); e != nil {
				log.Printf("panic in %s: %v", r.URL.Path, e)
				writeErr(w, 500, fmt.Errorf("internal error: %v", e))
			}
		}()
		h.ServeHTTP(w, r)
	})
}

var devDir string

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

// only accept requests addressed to the loopback host (DNS-rebinding guard)
func localOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if i := strings.LastIndex(host, ":"); i >= 0 {
			host = host[:i]
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "forbidden", 403)
			return
		}
		h.ServeHTTP(w, r)
	})
}

type designReq struct {
	Params  engine.Params   `json:"params"`
	Circuit *engine.Circuit `json:"circuit"`
}

func settingsStatus() map[string]any {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	mask := func(s string) string {
		if len(s) <= 6 {
			return strings.Repeat("•", len(s))
		}
		return s[:3] + strings.Repeat("•", 6) + s[len(s)-3:]
	}
	return map[string]any{
		"mouser":          settings.MouserKey != "",
		"digikey":         settings.DigikeyClientID != "" && settings.DigikeySecret != "",
		"mouserKey":       mask(settings.MouserKey),
		"digikeyClientId": mask(settings.DigikeyClientID),
		"digikeySecret":   mask(settings.DigikeySecret),
		"digikeySite":     settings.DigikeySite,
		"digikeyCurrency": settings.DigikeyCurrency,
		"inStockOnly":     settings.InStockOnly,
		"configDir":       cfgDir,
	}
}

func routes() http.Handler {
	mux := http.NewServeMux()
	var sub fs.FS
	if devDir != "" {
		// -dev: serve the UI straight from disk so edits show up on reload
		sub = os.DirFS(devDir)
	} else {
		sub, _ = fs.Sub(webFS, "web")
	}
	mux.Handle("/", noCache(http.FileServer(http.FS(sub))))

	mux.HandleFunc("/api/init", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"version":   version,
			"defaults":  map[string]engine.Params{"dc": engine.DefaultParams("dc"), "ac": engine.DefaultParams("ac")},
			"standards": engine.Standards,
			"settings":  settingsStatus(),
			"library":   len(engine.Library),
		})
	})

	mux.HandleFunc("/api/design", func(w http.ResponseWriter, r *http.Request) {
		var req designReq
		if err := readJSON(r, &req); err != nil {
			writeErr(w, 400, err)
			return
		}
		out, err := engine.Design(req.Params)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		sim, err := engine.Simulate(req.Params, out.Circuit)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, map[string]any{"circuit": out.Circuit, "log": out.Log, "sim": sim})
	})

	mux.HandleFunc("/api/simulate", func(w http.ResponseWriter, r *http.Request) {
		var req designReq
		if err := readJSON(r, &req); err != nil {
			writeErr(w, 400, err)
			return
		}
		if req.Circuit == nil {
			writeErr(w, 400, errors.New("no circuit"))
			return
		}
		sim, err := engine.Simulate(req.Params, req.Circuit)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, map[string]any{"sim": sim})
	})

	mux.HandleFunc("/api/parts", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Comp    engine.Comp `json:"comp"`
			Keyword string      `json:"keyword"`
			Sources []string    `json:"sources"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, 400, err)
			return
		}
		kw := strings.TrimSpace(req.Keyword)
		if kw == "" {
			kw = engine.SearchKeywords(req.Comp)
		}
		settingsMu.Lock()
		s := settings
		settingsMu.Unlock()
		type res struct {
			src string
			c   []parts.Candidate
			err error
		}
		ch := make(chan res, 3)
		n := 0
		for _, src := range req.Sources {
			switch src {
			case "library":
				n++
				go func() { ch <- res{"library", parts.FromLibrary(engine.Candidates(req.Comp)), nil} }()
			case "mouser":
				n++
				go func() { c, err := parts.MouserSearch(s, kw, 40); ch <- res{"mouser", c, err} }()
			case "digikey":
				n++
				go func() { c, err := parts.DigikeySearch(s, kw, 40); ch <- res{"digikey", c, err} }()
			}
		}
		all := []parts.Candidate{}
		errs := map[string]string{}
		counts := map[string]int{}
		for i := 0; i < n; i++ {
			x := <-ch
			if x.err != nil {
				errs[x.src] = x.err.Error()
				continue
			}
			counts[x.src] = len(x.c)
			all = append(all, x.c...)
		}
		all = parts.Rank(req.Comp, all)
		writeJSON(w, map[string]any{"keyword": kw, "results": all, "errors": errs, "counts": counts})
	})

	mux.HandleFunc("/api/applypart", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Comp engine.Comp     `json:"comp"`
			Cand parts.Candidate `json:"cand"`
		}
		if err := readJSON(r, &req); err != nil {
			writeErr(w, 400, err)
			return
		}
		c := parts.Apply(req.Comp, req.Cand)
		writeJSON(w, map[string]any{"comp": c})
	})

	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var in map[string]any
			if err := readJSON(r, &in); err != nil {
				writeErr(w, 400, err)
				return
			}
			settingsMu.Lock()
			str := func(k string, dst *string) {
				if v, ok := in[k].(string); ok && !strings.Contains(v, "•") {
					*dst = strings.TrimSpace(v)
				}
			}
			str("mouserKey", &settings.MouserKey)
			str("digikeyClientId", &settings.DigikeyClientID)
			str("digikeySecret", &settings.DigikeySecret)
			str("digikeySite", &settings.DigikeySite)
			str("digikeyCurrency", &settings.DigikeyCurrency)
			if v, ok := in["inStockOnly"].(bool); ok {
				settings.InStockOnly = v
			}
			err := saveSettings()
			settingsMu.Unlock()
			if err != nil {
				writeErr(w, 500, err)
				return
			}
		}
		writeJSON(w, settingsStatus())
	})

	mux.HandleFunc("/api/testkeys", func(w http.ResponseWriter, r *http.Request) {
		settingsMu.Lock()
		s := settings
		settingsMu.Unlock()
		out := map[string]string{}
		if s.MouserKey != "" {
			if c, err := parts.MouserSearch(s, "GRM188R71H104KA93D", 3); err != nil {
				out["mouser"] = err.Error()
			} else {
				out["mouser"] = fmt.Sprintf("OK (%d results)", len(c))
			}
		} else {
			out["mouser"] = "not configured"
		}
		if s.DigikeyClientID != "" {
			if c, err := parts.DigikeySearch(s, "GRM188R71H104KA93D", 3); err != nil {
				out["digikey"] = err.Error()
			} else {
				out["digikey"] = fmt.Sprintf("OK (%d results)", len(c))
			}
		} else {
			out["digikey"] = "not configured"
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("/api/export/spice", func(w http.ResponseWriter, r *http.Request) {
		var req designReq
		if err := readJSON(r, &req); err != nil || req.Circuit == nil {
			writeErr(w, 400, errors.New("bad request"))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, engine.SpiceNetlist(req.Params, req.Circuit))
	})

	mux.HandleFunc("/api/export/bom", func(w http.ResponseWriter, r *http.Request) {
		var req designReq
		if err := readJSON(r, &req); err != nil || req.Circuit == nil {
			writeErr(w, 400, errors.New("bad request"))
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = io.WriteString(w, "\xef\xbb\xbf"+engine.BOMCSV(req.Params, req.Circuit))
	})

	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		pingMu.Lock()
		lastPing = time.Now()
		quitAt = time.Time{}
		pingMu.Unlock()
		w.WriteHeader(204)
	})
	mux.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		// the window was closed (or reloaded): exit unless a new ping arrives
		pingMu.Lock()
		quitAt = time.Now().Add(6 * time.Second)
		pingMu.Unlock()
		w.WriteHeader(204)
	})
	return localOnly(recoverer(mux))
}

// ---------------------------------------------------------------------------
// Browser / app window

func findBrowser() string {
	switch runtime.GOOS {
	case "windows":
		var cands []string
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
			if b := os.Getenv(env); b != "" {
				cands = append(cands,
					filepath.Join(b, "Microsoft", "Edge", "Application", "msedge.exe"),
					filepath.Join(b, "Google", "Chrome", "Application", "chrome.exe"))
			}
		}
		for _, c := range cands {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	case "darwin":
		for _, c := range []string{"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"} {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	default:
		for _, c := range []string{"microsoft-edge", "google-chrome", "chromium", "chromium-browser"} {
			if p, err := exec.LookPath(c); err == nil {
				return p
			}
		}
	}
	return ""
}

func openUI(url string) {
	if b := findBrowser(); b != "" {
		prof := filepath.Join(cfgDir, "window")
		cmd := exec.Command(b, "--app="+url, "--user-data-dir="+prof, "--window-size=1500,960",
			"--no-first-run", "--no-default-browser-check", "--disable-features=Translate")
		if err := cmd.Start(); err == nil {
			log.Printf("opened app window with %s", b)
			return
		}
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open a browser: %v - open %s manually", err, url)
	}
}

func watchdog() {
	for {
		time.Sleep(2 * time.Second)
		pingMu.Lock()
		idle := time.Since(lastPing)
		q := quitAt
		pingMu.Unlock()
		if (!q.IsZero() && time.Now().After(q)) || idle > 180*time.Second {
			log.Println("UI closed - exiting")
			os.Exit(0)
		}
	}
}

func main() {
	port := flag.Int("port", 0, "port to listen on (0 = random)")
	noBrowser := flag.Bool("nobrowser", false, "do not open a window; keep running")
	dev := flag.Bool("dev", false, "developer mode: serve ./web from disk (edit + reload) and do not exit when the window closes")
	flag.Parse()
	if *dev {
		devDir = "web"
	}

	// Windows reads MIME types from the registry, which is sometimes wrong
	// for .js/.css - register them explicitly.
	for ext, t := range map[string]string{".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8",
		".html": "text/html; charset=utf-8", ".svg": "image/svg+xml", ".png": "image/png", ".ico": "image/x-icon"} {
		_ = mime.AddExtensionType(ext, t)
	}
	cfgDir = configDir()
	if lf, err := os.OpenFile(filepath.Join(cfgDir, "log.txt"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(lf, os.Stderr))
	}
	loadSettings()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatal(err)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)
	log.Printf("EMI Filter Designer %s listening on %s", version, url)
	fmt.Println("EMI Filter Designer running at", url)
	if !*noBrowser {
		go openUI(url)
		if !*dev {
			go watchdog()
		}
	}
	srv := &http.Server{Handler: routes(), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.Serve(ln))
}
