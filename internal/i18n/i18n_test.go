package i18n

import (
	"net/http/httptest"
	"testing"
)

func TestTextDefaultsToEnglish(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := Text(r, LoginDenied); got != "Sign-in was denied" {
		t.Errorf("Text ohne Accept-Language = %q, erwartet Englisch", got)
	}
}

func TestTextSwitchesToGerman(t *testing.T) {
	cases := []string{"de", "de-DE", "de-DE,en;q=0.8", "DE"}
	for _, header := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Accept-Language", header)
		if got := Text(r, LoginDenied); got != "Anmeldung abgelehnt" {
			t.Errorf("Accept-Language %q → %q, erwartet Deutsch", header, got)
		}
	}
}

func TestTextFallsBackToEnglishForOtherLanguages(t *testing.T) {
	cases := []string{"fr", "fr-FR,de;q=0.5", "en-US"}
	for _, header := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Accept-Language", header)
		if got := Text(r, LoginDenied); got != "Sign-in was denied" {
			t.Errorf("Accept-Language %q → %q, erwartet Englisch", header, got)
		}
	}
}

// TestMessagesAreComplete stellt sicher, dass keine Sprache einen Schlüssel
// vergisst — sonst würde Text() für diesen Fall stillschweigend "" liefern.
func TestMessagesAreComplete(t *testing.T) {
	keys := []string{
		LoginRandomUnavailable, LoginStartFailed, LoginFlowExpired, LoginDenied,
		LoginStateMismatch, LoginExchangeFailed, LoginNoIDToken, LoginInvalidToken,
		LoginNonceMismatch, LoginSessionFailed, QRRateLimited, QRTokenMissing,
		QRGenerateFailed,
	}
	for lang, table := range messages {
		if len(table) != len(keys) {
			t.Errorf("Sprache %q hat %d Einträge, erwartet %d", lang, len(table), len(keys))
		}
		for _, key := range keys {
			if table[key] == "" {
				t.Errorf("Sprache %q: Schlüssel %q fehlt oder ist leer", lang, key)
			}
		}
	}
}
