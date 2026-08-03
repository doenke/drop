// Package auth kümmert sich um Login per OIDC und um das signierte
// Session-Cookie, mit dem sich auch der WebSocket-Upgrade ausweist.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	// SessionCookie hält die Anmeldung; nur wer sie hat, darf Räume anlegen.
	SessionCookie = "drop_session"
	// flowCookie hält state, nonce und PKCE-Verifier für die Dauer des
	// Redirects zu Pocket ID und zurück.
	flowCookie = "drop_flow"
)

var (
	ErrNoSession      = errors.New("keine Session")
	ErrBadSignature   = errors.New("Signatur ungültig")
	ErrSessionExpired = errors.New("Session abgelaufen")
)

// Session ist der Inhalt des Cookies. Mehr als Subject, Anzeigename und die
// Adresse des Profilbilds braucht drop nicht — es gibt keine Rollen und keine
// Persistenz. Die Bildadresse steht hier, weil es sonst nirgends einen Ort
// gäbe, sie zwischen Login und Abruf zu halten.
type Session struct {
	Subject string `json:"sub"`
	Name    string `json:"name,omitempty"`
	Picture string `json:"pic,omitempty"`
	Expires int64  `json:"exp"`
}

// Signer signiert und prüft Cookie-Werte mit HMAC-SHA256.
type Signer struct {
	key    []byte
	ttl    time.Duration
	secure bool
}

func NewSigner(key []byte, ttl time.Duration, secure bool) *Signer {
	return &Signer{key: key, ttl: ttl, secure: secure}
}

// TTL ist die Lebensdauer neuer Sessions.
func (s *Signer) TTL() time.Duration { return s.ttl }

// sign hängt an den Payload eine Base64-URL-kodierte HMAC-Signatur an.
func (s *Signer) sign(payload []byte) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write(payload)
	enc := base64.RawURLEncoding
	return enc.EncodeToString(payload) + "." + enc.EncodeToString(mac.Sum(nil))
}

// verify prüft die Signatur in konstanter Zeit und gibt den Payload zurück.
func (s *Signer) verify(value string) ([]byte, error) {
	rawPayload, rawSig, ok := strings.Cut(value, ".")
	if !ok {
		return nil, ErrBadSignature
	}
	enc := base64.RawURLEncoding
	payload, err := enc.DecodeString(rawPayload)
	if err != nil {
		return nil, ErrBadSignature
	}
	sig, err := enc.DecodeString(rawSig)
	if err != nil {
		return nil, ErrBadSignature
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, ErrBadSignature
	}
	return payload, nil
}

// Issue setzt ein frisches Session-Cookie.
func (s *Signer) Issue(w http.ResponseWriter, sess Session) error {
	if sess.Expires == 0 {
		sess.Expires = time.Now().Add(s.ttl).Unix()
	}
	payload, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    s.sign(payload),
		Path:     "/",
		Expires:  time.Unix(sess.Expires, 0),
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// Read liest die Session aus dem Request. Fehlt sie oder ist sie ungültig,
// kommt ein Fehler zurück — der Aufrufer behandelt das als "nicht angemeldet".
func (s *Signer) Read(r *http.Request) (Session, error) {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return Session{}, ErrNoSession
	}
	payload, err := s.verify(c.Value)
	if err != nil {
		return Session{}, err
	}
	var sess Session
	if err := json.Unmarshal(payload, &sess); err != nil {
		return Session{}, ErrBadSignature
	}
	if time.Now().Unix() >= sess.Expires {
		return Session{}, ErrSessionExpired
	}
	return sess, nil
}

// Clear löscht das Session-Cookie.
func (s *Signer) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}
