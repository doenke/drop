// Package avatar liefert das Profilbild aus dem OIDC-Login über den eigenen
// Server aus, statt den Browser direkt zum Identity-Provider zu schicken.
package avatar

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/doenke/drop/internal/auth"
)

const (
	fetchTimeout = 10 * time.Second
	maxRedirects = 3
	// Profilbilder sind klein; alles darüber ist ein Fehler oder ein Angriff.
	defaultMaxBytes = 2 << 20
)

// Bildtypen, die ein Browser gefahrlos in ein <img> laden kann. SVG fehlt
// bewusst: es kann Skript enthalten und liefe hier unter eigener Herkunft.
var allowedTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
	"image/avif": true,
}

// Handler beantwortet GET /api/avatar mit dem Profilbild der eigenen Session.
//
// Der Umweg über den Server hat zwei Gründe: der Browser verrät dem Identity-
// Provider so nicht, wann jemand drop benutzt, und das Bild kommt unter
// eigener Herkunft, ohne Ausnahme in der Content-Security-Policy.
//
// Weil die Adresse aus einem Token stammt, ist der Abruf eine mögliche
// SSRF-Stelle. Deshalb wird nur von ausdrücklich erlaubten Hosts geladen —
// standardmäßig nur vom Issuer selbst —, und zwar bei jeder Weiterleitung neu
// geprüft.
type Handler struct {
	signer   *auth.Signer
	client   *http.Client
	allowed  map[string]bool
	maxBytes int64
	log      *slog.Logger
}

// New baut den Handler. allowedHosts sind Einträge der Form "host" oder
// "host:port" (ohne Schema). Ohne Portangabe gelten die Standard-Webports.
func New(signer *auth.Signer, allowedHosts []string, log *slog.Logger) *Handler {
	allowed := make(map[string]bool, 2*len(allowedHosts))
	for _, entry := range allowedHosts {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if host, port, err := net.SplitHostPort(entry); err == nil {
			allowed[host+":"+port] = true
			continue
		}
		// Ohne Port: beide Standard-Webports desselben Hosts erlauben.
		allowed[entry+":443"] = true
		allowed[entry+":80"] = true
	}
	h := &Handler{
		signer:   signer,
		allowed:  allowed,
		maxBytes: defaultMaxBytes,
		log:      log,
	}
	h.client = &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return http.ErrUseLastResponse
			}
			// Eine Weiterleitung darf nicht aus der Erlaubnisliste
			// herausführen.
			if !h.hostAllowed(req.URL) {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	return h
}

// hostAllowed vergleicht Host *und* Port. Nur den Hostnamen zu prüfen wäre zu
// grob: auf demselben Rechner wie der Identity-Provider können andere Dienste
// auf anderen Ports laufen, die niemand über diesen Umweg erreichen soll.
func (h *Handler) hostAllowed(u *url.URL) bool {
	if u.Scheme != "https" && u.Scheme != "http" {
		return false
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return h.allowed[strings.ToLower(u.Hostname())+":"+port]
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sess, err := h.signer.Read(r)
	if err != nil || sess.Picture == "" {
		http.NotFound(w, r)
		return
	}
	target, err := url.Parse(sess.Picture)
	if err != nil || !h.hostAllowed(target) {
		h.log.Debug("Profilbild-Adresse nicht erlaubt", "url", sess.Picture)
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	req.Header.Set("Accept", "image/*")

	resp, err := h.client.Do(req)
	if err != nil {
		h.log.Debug("Profilbild nicht abrufbar", "err", err)
		http.NotFound(w, r)
		return
	}
	defer resp.Body.Close()

	// Nach einer abgebrochenen Weiterleitung steht hier der 3xx selbst.
	if resp.StatusCode != http.StatusOK {
		http.NotFound(w, r)
		return
	}
	mediaType, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";")
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if !allowedTypes[mediaType] {
		h.log.Debug("Profilbild hat einen unerwarteten Typ", "type", mediaType)
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	// Privat und kurz: das Bild gehört zur Session, nicht zur Seite.
	w.Header().Set("Cache-Control", "private, max-age=300")

	// LimitReader deckelt auch dann, wenn die Gegenstelle eine falsche oder
	// gar keine Content-Length meldet.
	if _, err := io.Copy(w, io.LimitReader(resp.Body, h.maxBytes)); err != nil {
		h.log.Debug("Profilbild abgebrochen", "err", err)
	}
}
