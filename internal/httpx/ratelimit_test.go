package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiterBurstThenRefill(t *testing.T) {
	l := NewLimiter(60, 3) // ein Token pro Sekunde, Burst 3
	now := time.Now()
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !l.Allow("ip") {
			t.Fatalf("Versuch %d hätte im Burst erlaubt sein müssen", i+1)
		}
	}
	if l.Allow("ip") {
		t.Fatal("vierter Versuch hätte gebremst werden müssen")
	}

	// Eine Sekunde später ist genau ein Token nachgelaufen.
	now = now.Add(time.Second)
	if !l.Allow("ip") {
		t.Fatal("nach einer Sekunde hätte ein Versuch frei sein müssen")
	}
	if l.Allow("ip") {
		t.Fatal("es war nur ein Token nachgelaufen")
	}
}

func TestLimiterSeparatesKeys(t *testing.T) {
	l := NewLimiter(60, 1)
	if !l.Allow("a") || !l.Allow("b") {
		t.Fatal("verschiedene IPs teilen sich ein Budget")
	}
	if l.Allow("a") {
		t.Fatal("Budget von a war nicht aufgebraucht")
	}
}

func TestLimiterCollectDropsIdleKeys(t *testing.T) {
	l := NewLimiter(60, 2)
	now := time.Now()
	l.now = func() time.Time { return now }

	l.Allow("ip")
	now = now.Add(time.Hour)
	l.Collect()

	l.mu.Lock()
	n := len(l.states)
	l.mu.Unlock()
	if n != 0 {
		t.Fatalf("Limiter hält noch %d Einträge", n)
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:34567"
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")

	if got := ClientIP(r, true); got != "203.0.113.9" {
		t.Errorf("mit Proxy-Vertrauen: %q", got)
	}
	// Ohne Vertrauen darf der Header das Rate-Limit nicht aushebeln.
	if got := ClientIP(r, false); got != "10.0.0.5" {
		t.Errorf("ohne Proxy-Vertrauen: %q", got)
	}
}
