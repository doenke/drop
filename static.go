package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/doenke/drop/internal/config"
)

// asset ist eine eingebettete Datei mitsamt vorberechnetem ETag. Die Dateien
// sind klein und ändern sich zur Laufzeit nie, deshalb liegen sie komplett im
// Speicher und werden per Revalidierung ausgeliefert.
type asset struct {
	body        []byte
	etag        string
	contentType string
}

var assets = map[string]asset{}

func init() {
	err := fs.WalkDir(webFS, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(webFS, p)
		if err != nil {
			return err
		}
		ct := mime.TypeByExtension(path.Ext(p))
		if ct == "" {
			ct = "application/octet-stream"
		}
		assets[strings.TrimPrefix(p, "web/")] = newAsset(body, ct)
		return nil
	})
	if err != nil {
		panic("eingebettete Assets nicht lesbar: " + err.Error())
	}
}

func newAsset(body []byte, contentType string) asset {
	sum := sha256.Sum256(body)
	return asset{
		body:        body,
		etag:        `"` + hex.EncodeToString(sum[:8]) + `"`,
		contentType: contentType,
	}
}

// manifestDoc spiegelt web/manifest.json, damit applyBranding Name und
// Short-Name anpassen kann, ohne den Rest der Datei von Hand nachzubauen.
type manifestDoc struct {
	Name            string `json:"name"`
	ShortName       string `json:"short_name"`
	Description     string `json:"description"`
	Lang            string `json:"lang"`
	Dir             string `json:"dir"`
	StartURL        string `json:"start_url"`
	Scope           string `json:"scope"`
	Display         string `json:"display"`
	Orientation     string `json:"orientation"`
	BackgroundColor string `json:"background_color"`
	ThemeColor      string `json:"theme_color"`
	Icons           []struct {
		Src     string `json:"src"`
		Sizes   string `json:"sizes"`
		Type    string `json:"type"`
		Purpose string `json:"purpose,omitempty"`
	} `json:"icons"`
}

// brandingData sind die Werte, die index.html als Go-Template einsetzt.
type brandingData struct {
	Title         string
	HeaderLogoURL string
}

// applyBranding rendert index.html und manifest.json mit dem konfigurierten
// Titel und optionalen Kopfzeilen-Logo. Muss einmal nach dem Laden der
// Konfiguration laufen, bevor der Server Verbindungen annimmt — danach liest
// niemand mehr aus assets, während hier geschrieben wird.
//
// Die Quelle kommt bei jedem Aufruf frisch aus dem unveränderlichen webFS,
// nie aus der assets-Map: die enthält nach dem ersten Lauf schon das fertig
// gerenderte HTML ohne Platzhalter, ein zweiter Durchlauf daraus hätte also
// nichts mehr zum Ersetzen.
func applyBranding(cfg *config.Config) error {
	idxSource, err := fs.ReadFile(webFS, "web/index.html")
	if err != nil {
		return fmt.Errorf("index.html lesen: %w", err)
	}
	tmpl, err := template.New("index.html").Parse(string(idxSource))
	if err != nil {
		return fmt.Errorf("index.html als Template lesen: %w", err)
	}
	var buf bytes.Buffer
	data := brandingData{Title: cfg.Title, HeaderLogoURL: cfg.HeaderLogoURL}
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("index.html rendern: %w", err)
	}
	assets["index.html"] = newAsset(buf.Bytes(), assets["index.html"].contentType)

	manSource, err := fs.ReadFile(webFS, "web/manifest.json")
	if err != nil {
		return fmt.Errorf("manifest.json lesen: %w", err)
	}
	var manifest manifestDoc
	if err := json.Unmarshal(manSource, &manifest); err != nil {
		return fmt.Errorf("manifest.json lesen: %w", err)
	}
	manifest.Name = cfg.Title
	manifest.ShortName = cfg.Title
	out, err := json.MarshalIndent(&manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("manifest.json rendern: %w", err)
	}
	assets["manifest.json"] = newAsset(out, assets["manifest.json"].contentType)
	return nil
}

// mountStatic registriert die App-Shell und alle statischen Dateien. Die Shell
// hängt an "/" und an "/r/{token}", damit ein gescannter QR-Link direkt im
// Raum landet, ohne dass es dafür eine zweite HTML-Datei braucht.
func mountStatic(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", serveShell)
	mux.HandleFunc("GET /r/{token}", serveShell)
	mux.HandleFunc("GET /static/{file...}", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, r, r.PathValue("file"))
	})
	mux.HandleFunc("GET /icons/{file...}", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, r, "icons/"+r.PathValue("file"))
	})
	mux.HandleFunc("GET /manifest.json", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, r, "manifest.json")
	})
	// Der Service Worker muss vom Scope "/" aus ausgeliefert werden, sonst
	// darf er die ganze App nicht kontrollieren.
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, r, "sw.js")
	})
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, r, "icons/favicon.ico")
	})
}

func serveShell(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	serveAsset(w, r, "index.html")
}

func serveAsset(w http.ResponseWriter, r *http.Request, name string) {
	a, ok := assets[path.Clean(name)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", a.contentType)
	w.Header().Set("ETag", a.etag)
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(a.body))
}
