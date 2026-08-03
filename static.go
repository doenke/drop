package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
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
		sum := sha256.Sum256(body)
		ct := mime.TypeByExtension(path.Ext(p))
		if ct == "" {
			ct = "application/octet-stream"
		}
		assets[strings.TrimPrefix(p, "web/")] = asset{
			body:        body,
			etag:        `"` + hex.EncodeToString(sum[:8]) + `"`,
			contentType: ct,
		}
		return nil
	})
	if err != nil {
		panic("eingebettete Assets nicht lesbar: " + err.Error())
	}
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
