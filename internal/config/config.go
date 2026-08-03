// Package config liest die Laufzeit-Konfiguration aus der Umgebung.
package config

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config hält alle Einstellungen des Servers. Alles kommt aus Env-Variablen
// mit dem Präfix DROP_; Secrets stehen nie im Repo.
type Config struct {
	Addr      string
	PublicURL *url.URL

	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCScopes       []string

	// AvatarHosts sind die Hosts, von denen der Server Profilbilder holen
	// darf. Der Issuer steht immer drin; weitere kommen aus
	// DROP_AVATAR_HOSTS, falls der Provider die Bilder woanders ablegt.
	AvatarHosts []string

	SessionKey []byte
	SessionTTL time.Duration
	// SessionKeyEphemeral ist true, wenn kein DROP_SESSION_KEY gesetzt war
	// und der Schlüssel beim Start erzeugt wurde.
	SessionKeyEphemeral bool

	TrustProxy bool

	// Raum-Lifecycle
	RoomEmptyGrace time.Duration
	RoomMaxAge     time.Duration
	RoomMaxMembers int

	// Transfer-Limits
	MaxFileSize     int64
	MaxChunkSize    int64
	MaxTextItemSize int64
	MaxLiveTextSize int64

	// Join-Rate-Limit pro IP
	JoinRatePerMinute int
	JoinRateBurst     int
}

// Load liest die Konfiguration aus der Umgebung und validiert sie.
func Load() (*Config, error) {
	c := &Config{
		Addr:              env("DROP_ADDR", ":8080"),
		OIDCIssuer:        strings.TrimSuffix(env("DROP_OIDC_ISSUER", ""), "/"),
		OIDCClientID:      env("DROP_OIDC_CLIENT_ID", ""),
		OIDCClientSecret:  env("DROP_OIDC_CLIENT_SECRET", ""),
		OIDCScopes:        splitList(env("DROP_OIDC_SCOPES", "openid,profile,email")),
		SessionTTL:        envDuration("DROP_SESSION_TTL", 12*time.Hour),
		TrustProxy:        envBool("DROP_TRUSTED_PROXY", false),
		RoomEmptyGrace:    envDuration("DROP_ROOM_EMPTY_GRACE", 60*time.Second),
		RoomMaxAge:        envDuration("DROP_ROOM_MAX_AGE", 12*time.Hour),
		RoomMaxMembers:    envInt("DROP_ROOM_MAX_MEMBERS", 16),
		MaxFileSize:       envBytes("DROP_MAX_FILE_SIZE", 100<<20),
		MaxChunkSize:      envBytes("DROP_MAX_CHUNK_SIZE", 64<<10),
		MaxTextItemSize:   envBytes("DROP_MAX_TEXT_ITEM_SIZE", 256<<10),
		MaxLiveTextSize:   envBytes("DROP_MAX_LIVE_TEXT_SIZE", 64<<10),
		JoinRatePerMinute: envInt("DROP_JOIN_RATE_PER_MINUTE", 10),
		JoinRateBurst:     envInt("DROP_JOIN_RATE_BURST", 5),
	}

	raw := env("DROP_PUBLIC_URL", "")
	if raw == "" {
		return nil, errors.New("DROP_PUBLIC_URL ist nicht gesetzt (z. B. https://drop.kanonenwiese.de)")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("DROP_PUBLIC_URL ist keine absolute URL: %q", raw)
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	c.PublicURL = u

	if c.OIDCIssuer == "" || c.OIDCClientID == "" {
		return nil, errors.New("DROP_OIDC_ISSUER und DROP_OIDC_CLIENT_ID müssen gesetzt sein")
	}
	issuerURL, err := url.Parse(c.OIDCIssuer)
	if err != nil || issuerURL.Host == "" {
		return nil, fmt.Errorf("DROP_OIDC_ISSUER ist keine absolute URL: %q", c.OIDCIssuer)
	}
	c.AvatarHosts = append([]string{issuerURL.Host}, splitList(env("DROP_AVATAR_HOSTS", ""))...)

	if key := env("DROP_SESSION_KEY", ""); key != "" {
		if len(key) < 16 {
			return nil, errors.New("DROP_SESSION_KEY ist zu kurz (mindestens 16 Zeichen)")
		}
		c.SessionKey = []byte(key)
	} else {
		c.SessionKey = make([]byte, 32)
		if _, err := rand.Read(c.SessionKey); err != nil {
			return nil, fmt.Errorf("Session-Key erzeugen: %w", err)
		}
		c.SessionKeyEphemeral = true
	}

	if c.MaxChunkSize > c.MaxFileSize {
		return nil, errors.New("DROP_MAX_CHUNK_SIZE ist größer als DROP_MAX_FILE_SIZE")
	}
	if c.RoomMaxMembers < 2 {
		return nil, errors.New("DROP_ROOM_MAX_MEMBERS muss mindestens 2 sein")
	}

	return c, nil
}

// CookieSecure ist true, wenn die App unter https ausgeliefert wird. Hinter
// NPM mit Force-SSL ist das der Normalfall; für lokale Tests über http muss
// das Cookie ohne Secure-Flag gesetzt werden, sonst kommt es nie zurück.
func (c *Config) CookieSecure() bool { return c.PublicURL.Scheme == "https" }

// RedirectURL ist die Callback-URL, die in Pocket ID hinterlegt wird.
func (c *Config) RedirectURL() string { return c.PublicURL.String() + "/auth/callback" }

// RoomURL baut den Link, den der QR-Code trägt.
func (c *Config) RoomURL(token string) string {
	return c.PublicURL.String() + "/r/" + token
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, err := strconv.ParseBool(env(key, ""))
	if err != nil {
		return def
	}
	return v
}

func envInt(key string, def int) int {
	v, err := strconv.Atoi(env(key, ""))
	if err != nil {
		return def
	}
	return v
}

func envDuration(key string, def time.Duration) time.Duration {
	v, err := time.ParseDuration(env(key, ""))
	if err != nil {
		return def
	}
	return v
}

// envBytes akzeptiert reine Zahlen sowie die Suffixe k/m/g (binär).
func envBytes(key string, def int64) int64 {
	raw := strings.ToLower(strings.TrimSpace(env(key, "")))
	if raw == "" {
		return def
	}
	mult := int64(1)
	switch {
	case strings.HasSuffix(raw, "g"):
		mult, raw = 1<<30, raw[:len(raw)-1]
	case strings.HasSuffix(raw, "m"):
		mult, raw = 1<<20, raw[:len(raw)-1]
	case strings.HasSuffix(raw, "k"):
		mult, raw = 1<<10, raw[:len(raw)-1]
	}
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || v <= 0 {
		return def
	}
	return v * mult
}

func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
