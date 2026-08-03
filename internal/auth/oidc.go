package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// flowState überbrückt den Redirect zu Pocket ID. Es liegt signiert im
// Browser statt im Server-Speicher, damit ein Neustart laufende Logins nicht
// kaputt macht und der Server keinen Zustand halten muss.
type flowState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Next     string `json:"r,omitempty"`
	Expires  int64  `json:"exp"`
}

// OIDC bündelt den Login-Flow gegen den Identity-Provider (Pocket ID).
type OIDC struct {
	signer   *Signer
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
	log      *slog.Logger
}

// NewOIDC holt die Provider-Metadaten per Discovery. Ist der Provider beim
// Start nicht erreichbar, schlägt das hier fehl — das ist gewollt, sonst
// merkt man es erst beim ersten Login.
func NewOIDC(ctx context.Context, issuer, clientID, clientSecret, redirectURL string, scopes []string, signer *Signer, log *slog.Logger) (*OIDC, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("OIDC-Discovery für %s: %w", issuer, err)
	}
	return &OIDC{
		signer:   signer,
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
		oauth: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  redirectURL,
			Scopes:       scopes,
		},
		log: log,
	}, nil
}

// Mount registriert die drei Login-Routen.
func (o *OIDC) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", o.handleLogin)
	mux.HandleFunc("GET /auth/callback", o.handleCallback)
	mux.HandleFunc("POST /auth/logout", o.handleLogout)
}

func (o *OIDC) handleLogin(w http.ResponseWriter, r *http.Request) {
	state, err1 := randomToken(24)
	nonce, err2 := randomToken(24)
	if err1 != nil || err2 != nil {
		http.Error(w, "Zufallszahlen nicht verfügbar", http.StatusInternalServerError)
		return
	}
	verifier := oauth2.GenerateVerifier()

	flow := flowState{
		State:    state,
		Nonce:    nonce,
		Verifier: verifier,
		Next:     safeNext(r.URL.Query().Get("next")),
		Expires:  time.Now().Add(10 * time.Minute).Unix(),
	}
	if err := o.setFlowCookie(w, flow); err != nil {
		http.Error(w, "Login konnte nicht gestartet werden", http.StatusInternalServerError)
		return
	}

	url := o.oauth.AuthCodeURL(state,
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	)
	http.Redirect(w, r, url, http.StatusFound)
}

func (o *OIDC) handleCallback(w http.ResponseWriter, r *http.Request) {
	flow, err := o.readFlowCookie(r)
	o.clearFlowCookie(w)
	if err != nil {
		http.Error(w, "Login-Vorgang abgelaufen, bitte erneut versuchen", http.StatusBadRequest)
		return
	}
	if desc := r.URL.Query().Get("error"); desc != "" {
		o.log.Warn("OIDC-Provider meldet Fehler", "error", desc)
		http.Error(w, "Anmeldung abgelehnt", http.StatusForbidden)
		return
	}
	// Konstantzeit-Vergleich ist hier unnötig: der State ist kein Geheimnis,
	// sondern nur der CSRF-Schutz des Redirects.
	if r.URL.Query().Get("state") != flow.State {
		http.Error(w, "State stimmt nicht", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	tok, err := o.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		o.log.Warn("Token-Exchange fehlgeschlagen", "err", err)
		http.Error(w, "Anmeldung fehlgeschlagen", http.StatusBadGateway)
		return
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		http.Error(w, "Kein ID-Token erhalten", http.StatusBadGateway)
		return
	}
	idToken, err := o.verifier.Verify(ctx, rawID)
	if err != nil {
		o.log.Warn("ID-Token ungültig", "err", err)
		http.Error(w, "Anmeldung fehlgeschlagen", http.StatusForbidden)
		return
	}
	if idToken.Nonce != flow.Nonce {
		http.Error(w, "Nonce stimmt nicht", http.StatusForbidden)
		return
	}

	var claims struct {
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Email             string `json:"email"`
		Picture           string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		o.log.Warn("Claims nicht lesbar", "err", err)
	}

	sess := Session{
		Subject: idToken.Subject,
		Name:    displayName(claims.Name, claims.PreferredUsername, claims.Email),
		Picture: pictureURL(claims.Picture),
	}
	if err := o.signer.Issue(w, sess); err != nil {
		http.Error(w, "Session konnte nicht gesetzt werden", http.StatusInternalServerError)
		return
	}

	next := flow.Next
	if next == "" {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusFound)
}

func (o *OIDC) handleLogout(w http.ResponseWriter, r *http.Request) {
	o.signer.Clear(w)
	w.WriteHeader(http.StatusNoContent)
}

func (o *OIDC) setFlowCookie(w http.ResponseWriter, flow flowState) error {
	payload, err := json.Marshal(flow)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     flowCookie,
		Value:    o.signer.sign(payload),
		Path:     "/auth",
		MaxAge:   int((10 * time.Minute).Seconds()),
		HttpOnly: true,
		Secure:   o.signer.secure,
		// Lax reicht: der Provider schickt den Browser per GET zurück.
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (o *OIDC) readFlowCookie(r *http.Request) (flowState, error) {
	c, err := r.Cookie(flowCookie)
	if err != nil {
		return flowState{}, ErrNoSession
	}
	payload, err := o.signer.verify(c.Value)
	if err != nil {
		return flowState{}, err
	}
	var flow flowState
	if err := json.Unmarshal(payload, &flow); err != nil {
		return flowState{}, ErrBadSignature
	}
	if time.Now().Unix() >= flow.Expires {
		return flowState{}, ErrSessionExpired
	}
	return flow, nil
}

func (o *OIDC) clearFlowCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     flowCookie,
		Value:    "",
		Path:     "/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   o.signer.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// MeHandler liefert dem Frontend den Anmeldestatus.
func (s *Signer) MeHandler(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Authenticated bool   `json:"authenticated"`
		Name          string `json:"name,omitempty"`
		Avatar        bool   `json:"avatar"`
	}{}
	if sess, err := s.Read(r); err == nil {
		resp.Authenticated = true
		resp.Name = sess.Name
		// Die Adresse selbst bleibt im Cookie; das Frontend holt das Bild
		// über den eigenen Proxy und erfährt nie, wo es liegt.
		resp.Avatar = sess.Picture != ""
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

// safeNext akzeptiert nur app-interne Pfade als Redirect-Ziel — sonst wäre
// der Login ein offener Redirector.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return ""
	}
	return next
}

// pictureURL nimmt den picture-Claim nur an, wenn er eine absolute http(s)-URL
// ist. Ob der Host abgerufen werden darf, entscheidet später der Proxy — hier
// wird nur aussortiert, was gar keine Bildadresse sein kann (data:, javascript:,
// relative Pfade).
func pictureURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	return u.String()
}

func displayName(candidates ...string) string {
	for _, c := range candidates {
		if c = strings.TrimSpace(c); c != "" {
			return c
		}
	}
	return "Angemeldet"
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
