// Package httpx enthält kleine HTTP-Hilfen, die zu keinem der Fachpakete
// gehören.
package httpx

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limiter ist ein Token-Bucket pro Client-IP. Er sichert die Beitrittsversuche
// ab: der 3-Wörter-Code hat bewusst wenig Entropie, deshalb darf niemand
// schnell viele Codes durchprobieren.
type Limiter struct {
	rate   float64 // Tokens pro Sekunde
	burst  float64
	mu     sync.Mutex
	states map[string]*bucket
	now    func() time.Time
}

type bucket struct {
	tokens float64
	seen   time.Time
}

// NewLimiter erlaubt perMinute Versuche pro Minute mit dem angegebenen Burst.
func NewLimiter(perMinute, burst int) *Limiter {
	if perMinute < 1 {
		perMinute = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &Limiter{
		rate:   float64(perMinute) / 60,
		burst:  float64(burst),
		states: map[string]*bucket{},
		now:    time.Now,
	}
}

// Allow zieht ein Token für den Key ab und sagt, ob der Versuch erlaubt ist.
func (l *Limiter) Allow(key string) bool {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.states[key]
	if !ok {
		b = &bucket{tokens: l.burst}
		l.states[key] = b
	} else {
		b.tokens = minFloat(l.burst, b.tokens+now.Sub(b.seen).Seconds()*l.rate)
	}
	b.seen = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Collect wirft Einträge weg, die lange genug ungenutzt sind, um wieder voll
// aufgefüllt zu sein — ohne das würde die Map unbegrenzt wachsen.
func (l *Limiter) Collect() {
	cutoff := time.Duration(l.burst/l.rate) * time.Second
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.states {
		if now.Sub(b.seen) > cutoff {
			delete(l.states, k)
		}
	}
}

// Run räumt regelmäßig auf, bis der Kanal schließt.
func (l *Limiter) Run(done <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			l.Collect()
		}
	}
}

// ClientIP ermittelt die Adresse des Clients. Hinter dem Nginx Proxy Manager
// steht die echte IP in X-Forwarded-For; ohne trustProxy wäre dieser Header
// frei fälschbar und damit als Rate-Limit-Key wertlos.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			first, _, _ := strings.Cut(fwd, ",")
			if ip := strings.TrimSpace(first); ip != "" {
				return ip
			}
		}
		if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
			return real
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
