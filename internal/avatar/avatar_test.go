package avatar

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/doenke/drop/internal/auth"
)

// pngBytes ist ein winziges, gültiges PNG.
var pngBytes = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89,
}

func newSigner() *auth.Signer {
	return auth.NewSigner([]byte("0123456789abcdef0123456789abcdef"), time.Hour, false)
}

// request baut eine Anfrage mit einer Session, die auf picture zeigt.
func request(t *testing.T, signer *auth.Signer, picture string) *http.Request {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := signer.Issue(rec, auth.Session{Subject: "user-1", Name: "Test", Picture: picture}); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/avatar", nil)
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}
	return r
}

// upstream simuliert den Identity-Provider.
func upstream(t *testing.T, handler http.HandlerFunc) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	// Host samt Port: der Handler prüft beides.
	return srv, u.Host
}

func serve(h *Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestServesImageFromAllowedHost(t *testing.T) {
	srv, host := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	})
	signer := newSigner()
	h := New(signer, []string{host}, discard())

	rec := serve(h, request(t, signer, srv.URL+"/avatar.png"))
	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), pngBytes) {
		t.Fatalf("Bild kam verändert an")
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "private") {
		t.Errorf("Cache-Control = %q, sollte privat sein", got)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("nosniff fehlt")
	}
}

func TestWithoutSessionOrPicture(t *testing.T) {
	signer := newSigner()
	h := New(signer, []string{"id.example"}, discard())

	// Gar keine Session.
	if rec := serve(h, httptest.NewRequest(http.MethodGet, "/api/avatar", nil)); rec.Code != http.StatusNotFound {
		t.Errorf("ohne Session: Status = %d", rec.Code)
	}
	// Angemeldet, aber der Provider hat kein Bild geliefert.
	if rec := serve(h, request(t, signer, "")); rec.Code != http.StatusNotFound {
		t.Errorf("ohne Bild: Status = %d", rec.Code)
	}
}

// TestRejectsForeignHost ist der eigentliche Schutz: die Adresse stammt aus
// einem Token, der Abruf wäre sonst eine SSRF-Stelle.
func TestRejectsForeignHost(t *testing.T) {
	var reached bool
	srv, host := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	})
	signer := newSigner()
	// Der Upstream ist gerade nicht erlaubt.
	h := New(signer, []string{"id.example"}, discard())
	_ = host

	if rec := serve(h, request(t, signer, srv.URL+"/avatar.png")); rec.Code != http.StatusNotFound {
		t.Fatalf("Status = %d, erwartet 404", rec.Code)
	}
	if reached {
		t.Fatal("der fremde Host wurde trotzdem angefragt")
	}
}

// Auf dem Rechner des Identity-Providers können weitere Dienste auf anderen
// Ports lauschen. Nur den Hostnamen zu prüfen würde die alle freigeben.
func TestRejectsSameHostOtherPort(t *testing.T) {
	var reached bool
	nachbar, nachbarHost := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	})
	_, idpHost := upstream(t, func(w http.ResponseWriter, r *http.Request) {})
	if hostOf(t, nachbarHost) != hostOf(t, idpHost) {
		t.Skipf("Testserver liegen auf verschiedenen Hosts: %s / %s", nachbarHost, idpHost)
	}

	signer := newSigner()
	// Erlaubt ist nur der Identity-Provider — der Nachbardienst nicht.
	h := New(signer, []string{idpHost}, discard())

	if rec := serve(h, request(t, signer, nachbar.URL+"/avatar.png")); rec.Code != http.StatusNotFound {
		t.Fatalf("Status = %d, erwartet 404", rec.Code)
	}
	if reached {
		t.Fatal("ein anderer Port auf demselben Host wurde erreicht")
	}
}

func hostOf(t *testing.T, hostPort string) string {
	t.Helper()
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", hostPort, err)
	}
	return host
}

func TestRejectsNonImageContentType(t *testing.T) {
	srv, host := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<script>alert(1)</script>"))
	})
	signer := newSigner()
	h := New(signer, []string{host}, discard())

	if rec := serve(h, request(t, signer, srv.URL+"/x")); rec.Code != http.StatusNotFound {
		t.Fatalf("Status = %d, erwartet 404", rec.Code)
	}
}

// SVG kann Skript enthalten und liefe unter eigener Herkunft — deshalb nicht.
func TestRejectsSVG(t *testing.T) {
	srv, host := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	})
	signer := newSigner()
	h := New(signer, []string{host}, discard())

	if rec := serve(h, request(t, signer, srv.URL+"/x.svg")); rec.Code != http.StatusNotFound {
		t.Fatalf("Status = %d, erwartet 404", rec.Code)
	}
}

func TestUpstreamErrorBecomesNotFound(t *testing.T) {
	srv, host := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "weg", http.StatusInternalServerError)
	})
	signer := newSigner()
	h := New(signer, []string{host}, discard())

	if rec := serve(h, request(t, signer, srv.URL+"/x")); rec.Code != http.StatusNotFound {
		t.Fatalf("Status = %d, erwartet 404", rec.Code)
	}
}

func TestBodyIsCapped(t *testing.T) {
	srv, host := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		// Deutlich mehr, als das Limit erlaubt, und ohne Content-Length.
		for i := 0; i < 64; i++ {
			_, _ = w.Write(bytes.Repeat([]byte{0}, 1024))
		}
	})
	signer := newSigner()
	h := New(signer, []string{host}, discard())
	h.maxBytes = 4096

	rec := serve(h, request(t, signer, srv.URL+"/gross.png"))
	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d", rec.Code)
	}
	if got := rec.Body.Len(); int64(got) != h.maxBytes {
		t.Fatalf("es kamen %d Bytes durch, gedeckelt war %d", got, h.maxBytes)
	}
}

// Eine Weiterleitung darf nicht aus der Erlaubnisliste herausführen.
func TestRedirectOffAllowedHostIsNotFollowed(t *testing.T) {
	var reached bool
	foreign, _ := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	})
	srv, host := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL+"/avatar.png", http.StatusFound)
	})
	signer := newSigner()
	h := New(signer, []string{host}, discard())

	if rec := serve(h, request(t, signer, srv.URL+"/x.png")); rec.Code != http.StatusNotFound {
		t.Fatalf("Status = %d, erwartet 404", rec.Code)
	}
	if reached {
		t.Fatal("der Weiterleitung zu einem fremden Host wurde gefolgt")
	}
}

func TestRedirectWithinAllowedHostIsFollowed(t *testing.T) {
	var mux http.ServeMux
	srv := httptest.NewServer(&mux)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ziel.png", http.StatusFound)
	})
	mux.HandleFunc("/ziel.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	})

	signer := newSigner()
	h := New(signer, []string{u.Host}, discard())

	rec := serve(h, request(t, signer, srv.URL+"/start"))
	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), pngBytes) {
		t.Fatal("Bild kam nicht durch")
	}
}

func TestRejectsNonHTTPScheme(t *testing.T) {
	signer := newSigner()
	h := New(signer, []string{"id.example"}, discard())
	for _, picture := range []string{
		"file:///etc/passwd",
		"gopher://id.example/1",
	} {
		if rec := serve(h, request(t, signer, picture)); rec.Code != http.StatusNotFound {
			t.Errorf("%q: Status = %d, erwartet 404", picture, rec.Code)
		}
	}
}

func TestReadAllForCoverage(t *testing.T) {
	// Stellt sicher, dass der Body wirklich geschlossen und lesbar ist.
	srv, host := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte{0xff, 0xd8, 0xff})
	})
	signer := newSigner()
	h := New(signer, []string{host}, discard())

	rec := serve(h, request(t, signer, srv.URL+"/a.jpg"))
	body, err := io.ReadAll(rec.Body)
	if err != nil || len(body) != 3 {
		t.Fatalf("Body = %v, err = %v", body, err)
	}
}
