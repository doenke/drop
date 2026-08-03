package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestSigner() *Signer {
	return NewSigner([]byte("0123456789abcdef0123456789abcdef"), time.Hour, false)
}

// requestWithCookies spiegelt die Set-Cookie-Header einer Antwort in einen
// neuen Request zurück, so wie es ein Browser täte.
func requestWithCookies(rec *httptest.ResponseRecorder) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}
	return r
}

func TestIssueAndRead(t *testing.T) {
	s := newTestSigner()
	rec := httptest.NewRecorder()
	if err := s.Issue(rec, Session{Subject: "user-1", Name: "Sönke"}); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	got, err := s.Read(requestWithCookies(rec))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Subject != "user-1" || got.Name != "Sönke" {
		t.Fatalf("unerwartete Session: %+v", got)
	}
	if got.Expires <= time.Now().Unix() {
		t.Fatalf("Ablauf liegt nicht in der Zukunft: %d", got.Expires)
	}
}

func TestReadRejectsTamperedPayload(t *testing.T) {
	s := newTestSigner()
	rec := httptest.NewRecorder()
	if err := s.Issue(rec, Session{Subject: "user-1"}); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	cookie := rec.Result().Cookies()[0]
	// Ein einzelnes Zeichen im Payload umbiegen muss die Signatur brechen.
	payload, sig, _ := strings.Cut(cookie.Value, ".")
	cookie.Value = flipFirstRune(payload) + "." + sig

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(cookie)
	if _, err := s.Read(r); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("erwartet ErrBadSignature, bekommen: %v", err)
	}
}

func TestReadRejectsForeignKey(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := newTestSigner().Issue(rec, Session{Subject: "user-1"}); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	other := NewSigner([]byte("ffffffffffffffffffffffffffffffff"), time.Hour, false)
	if _, err := other.Read(requestWithCookies(rec)); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("erwartet ErrBadSignature, bekommen: %v", err)
	}
}

func TestReadRejectsExpired(t *testing.T) {
	s := newTestSigner()
	rec := httptest.NewRecorder()
	if err := s.Issue(rec, Session{Subject: "user-1", Expires: time.Now().Add(-time.Minute).Unix()}); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := s.Read(requestWithCookies(rec)); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("erwartet ErrSessionExpired, bekommen: %v", err)
	}
}

func TestReadWithoutCookie(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := newTestSigner().Read(r); !errors.Is(err, ErrNoSession) {
		t.Fatalf("erwartet ErrNoSession, bekommen: %v", err)
	}
}

func TestClearExpiresCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestSigner().Clear(rec)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != SessionCookie || cookies[0].MaxAge >= 0 {
		t.Fatalf("Cookie wurde nicht gelöscht: %+v", cookies)
	}
}

func TestSafeNext(t *testing.T) {
	cases := map[string]string{
		"":                     "",
		"/r/abc":               "/r/abc",
		"//evil.example":       "",
		"https://evil.example": "",
		"javascript:alert(1)":  "",
		"/r/abc?x=1":           "/r/abc?x=1",
	}
	for in, want := range cases {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, erwartet %q", in, got, want)
		}
	}
}

func flipFirstRune(s string) string {
	if s == "" {
		return s
	}
	if s[0] == 'a' {
		return "b" + s[1:]
	}
	return "a" + s[1:]
}
