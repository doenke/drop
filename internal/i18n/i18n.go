// Package i18n liefert die wenigen Texte für rohe Browser-Fehlerseiten
// außerhalb der SPA (OIDC-Login-Flow, QR-Endpoint) auf Englisch oder
// Deutsch. Der Rest der App übersetzt clientseitig in web/i18n.js; hier
// gibt es keinen Client, der das übernehmen könnte, deshalb entscheidet der
// Accept-Language-Header.
package i18n

import (
	"net/http"
	"strings"
)

// Schlüssel für die einzelnen Fehlertexte.
const (
	LoginRandomUnavailable = "login-random-unavailable"
	LoginStartFailed       = "login-start-failed"
	LoginFlowExpired       = "login-flow-expired"
	LoginDenied            = "login-denied"
	LoginStateMismatch     = "login-state-mismatch"
	LoginExchangeFailed    = "login-exchange-failed"
	LoginNoIDToken         = "login-no-id-token"
	LoginInvalidToken      = "login-invalid-token"
	LoginNonceMismatch     = "login-nonce-mismatch"
	LoginSessionFailed     = "login-session-failed"

	QRRateLimited    = "qr-rate-limited"
	QRTokenMissing   = "qr-token-missing"
	QRGenerateFailed = "qr-generate-failed"
)

var messages = map[string]map[string]string{
	"en": {
		LoginRandomUnavailable: "Random numbers unavailable",
		LoginStartFailed:       "Could not start login",
		LoginFlowExpired:       "Login expired, please try again",
		LoginDenied:            "Sign-in was denied",
		LoginStateMismatch:     "State does not match",
		LoginExchangeFailed:    "Sign-in failed",
		LoginNoIDToken:         "No ID token received",
		LoginInvalidToken:      "Sign-in failed",
		LoginNonceMismatch:     "Nonce does not match",
		LoginSessionFailed:     "Could not set session",

		QRRateLimited:    "Too many requests",
		QRTokenMissing:   "Token missing",
		QRGenerateFailed: "Could not generate QR code",
	},
	"de": {
		LoginRandomUnavailable: "Zufallszahlen nicht verfügbar",
		LoginStartFailed:       "Login konnte nicht gestartet werden",
		LoginFlowExpired:       "Login-Vorgang abgelaufen, bitte erneut versuchen",
		LoginDenied:            "Anmeldung abgelehnt",
		LoginStateMismatch:     "State stimmt nicht",
		LoginExchangeFailed:    "Anmeldung fehlgeschlagen",
		LoginNoIDToken:         "Kein ID-Token erhalten",
		LoginInvalidToken:      "Anmeldung fehlgeschlagen",
		LoginNonceMismatch:     "Nonce stimmt nicht",
		LoginSessionFailed:     "Session konnte nicht gesetzt werden",

		QRRateLimited:    "Zu viele Anfragen",
		QRTokenMissing:   "Token fehlt",
		QRGenerateFailed: "QR-Code konnte nicht erzeugt werden",
	},
}

// Text liefert den Text zu key in der Sprache, die der Accept-Language-Header
// des Requests anfordert. Englisch ist der Standard; nur ein Header, dessen
// erster Tag mit "de" beginnt, schaltet auf Deutsch um. Bei nur zwei
// Sprachen lohnt sich kein Q-Value-Parsing — der erste Tag reicht.
func Text(r *http.Request, key string) string {
	lang := "en"
	if header := r.Header.Get("Accept-Language"); header != "" {
		tag := strings.TrimSpace(strings.SplitN(header, ",", 2)[0])
		tag = strings.TrimSpace(strings.SplitN(tag, ";", 2)[0])
		if strings.HasPrefix(strings.ToLower(tag), "de") {
			lang = "de"
		}
	}
	return messages[lang][key]
}
