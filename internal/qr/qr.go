// Package qr liefert den QR-Code zu einem Raum als PNG.
package qr

import (
	"net/http"

	"github.com/skip2/go-qrcode"

	"github.com/doenke/drop/internal/httpx"
	"github.com/doenke/drop/internal/i18n"
	"github.com/doenke/drop/internal/room"
)

const pngSize = 512

// Handler beantwortet GET /api/qr?token=… mit dem QR-Bild für den Raum-Link.
// Als Ausweis dient der Token selbst: wer ihn hat, könnte ohnehin beitreten.
// Trotzdem läuft die Anfrage über dasselbe Rate-Limit wie ein Beitritt, damit
// der Endpoint kein bequemer Weg zum Durchprobieren wird.
type Handler struct {
	hub        *room.Hub
	limiter    *httpx.Limiter
	roomURL    func(token string) string
	trustProxy bool
}

func New(hub *room.Hub, limiter *httpx.Limiter, roomURL func(string) string, trustProxy bool) *Handler {
	return &Handler{hub: hub, limiter: limiter, roomURL: roomURL, trustProxy: trustProxy}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.limiter.Allow(httpx.ClientIP(r, h.trustProxy)) {
		http.Error(w, i18n.Text(r, i18n.QRRateLimited), http.StatusTooManyRequests)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, i18n.Text(r, i18n.QRTokenMissing), http.StatusBadRequest)
		return
	}
	if _, err := h.hub.ByToken(token); err != nil {
		http.NotFound(w, r)
		return
	}

	png, err := qrcode.Encode(h.roomURL(token), qrcode.Medium, pngSize)
	if err != nil {
		http.Error(w, i18n.Text(r, i18n.QRGenerateFailed), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	// Der Code gehört zu einem ephemeren Raum und darf nirgends liegen bleiben.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}
