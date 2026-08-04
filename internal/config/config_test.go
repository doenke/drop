package config

import "testing"

// baseEnv setzt die drei Pflichtwerte, ohne die Load() gar nicht erst
// startet, damit jeder Test nur die für ihn relevante Variable dazusetzt.
func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DROP_PUBLIC_URL", "https://drop.example.com")
	t.Setenv("DROP_OIDC_ISSUER", "https://id.example.com")
	t.Setenv("DROP_OIDC_CLIENT_ID", "drop")
}

func TestTitleDefaultsToDrop(t *testing.T) {
	baseEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Title != "drop" {
		t.Errorf("Title = %q, erwartet \"drop\"", c.Title)
	}
}

func TestTitleFromEnv(t *testing.T) {
	baseEnv(t)
	t.Setenv("DROP_TITLE", "TeamDrop")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Title != "TeamDrop" {
		t.Errorf("Title = %q, erwartet \"TeamDrop\"", c.Title)
	}
}

func TestTitleWhitespaceOnlyFallsBackToDefault(t *testing.T) {
	baseEnv(t)
	// Nur der env()-Helfer behandelt eine leere Zeichenkette als "nicht
	// gesetzt"; reine Leerzeichen kämen ohne das Trim+Fallback als Titel
	// durch und die Kopfzeile stünde leer da.
	t.Setenv("DROP_TITLE", "   ")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Title != "drop" {
		t.Errorf("Title = %q, erwartet Fallback \"drop\"", c.Title)
	}
}

func TestHeaderLogoURLDefaultsToEmpty(t *testing.T) {
	baseEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HeaderLogoURL != "" {
		t.Errorf("HeaderLogoURL = %q, erwartet leer", c.HeaderLogoURL)
	}
}

func TestHeaderLogoURLAccepted(t *testing.T) {
	baseEnv(t)
	t.Setenv("DROP_HEADER_LOGO_URL", "https://static.example.com/logo.png")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HeaderLogoURL != "https://static.example.com/logo.png" {
		t.Errorf("HeaderLogoURL = %q", c.HeaderLogoURL)
	}
}

func TestHeaderLogoURLRejectsInvalid(t *testing.T) {
	baseEnv(t)
	for _, raw := range []string{
		"nicht-mal-eine-url",
		"javascript:alert(1)",
		"ftp://static.example.com/logo.png",
		"/nur/ein/pfad.png",
	} {
		t.Setenv("DROP_HEADER_LOGO_URL", raw)
		if _, err := Load(); err == nil {
			t.Errorf("DROP_HEADER_LOGO_URL=%q wurde akzeptiert, sollte aber am Start scheitern", raw)
		}
	}
}
