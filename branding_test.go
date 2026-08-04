package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/doenke/drop/internal/config"
)

func TestApplyBrandingDefaults(t *testing.T) {
	if err := applyBranding(&config.Config{Title: "drop"}); err != nil {
		t.Fatalf("applyBranding: %v", err)
	}

	html := string(assets["index.html"].body)
	if !strings.Contains(html, "<title>drop</title>") {
		t.Error("<title> trägt nicht den Titel")
	}
	if !strings.Contains(html, `class="brand-name">drop<`) {
		t.Error("Kopfzeile trägt nicht den Titel")
	}
	if strings.Contains(html, "header-logo") {
		t.Error("ohne HeaderLogoURL taucht trotzdem ein Logo-Element auf")
	}
	if !strings.Contains(html, `href="https://github.com/doenke/drop"`) {
		t.Error("Footer verlinkt nicht auf das GitHub-Projekt")
	}
	// Das Icon in der Kopfzeile muss dasselbe sein wie der Favicon —
	// sonst zeigen Tab und Kopfzeile zwei verschiedene Icons.
	if !strings.Contains(html, `class="brand-mark" src="/icons/icon.svg"`) {
		t.Error("Kopfzeilen-Icon zeigt nicht auf die zentrale icon.svg")
	}
	if !strings.Contains(html, `href="/icons/icon.svg" type="image/svg+xml"`) {
		t.Error("Favicon-Link fehlt oder zeigt auf eine andere Datei")
	}

	var manifest manifestDoc
	if err := json.Unmarshal(assets["manifest.json"].body, &manifest); err != nil {
		t.Fatalf("manifest.json ist kein gültiges JSON: %v", err)
	}
	if manifest.Name != "drop" || manifest.ShortName != "drop" {
		t.Errorf("Manifest-Name = %q/%q, erwartet \"drop\"/\"drop\"", manifest.Name, manifest.ShortName)
	}
	if len(manifest.Icons) == 0 {
		t.Error("Icons sind beim Rendern verlorengegangen")
	}
}

func TestApplyBrandingCustomTitleAndLogo(t *testing.T) {
	cfg := &config.Config{Title: "TeamDrop", HeaderLogoURL: "https://static.example.com/logo.png"}
	if err := applyBranding(cfg); err != nil {
		t.Fatalf("applyBranding: %v", err)
	}

	html := string(assets["index.html"].body)
	if !strings.Contains(html, "<title>TeamDrop</title>") {
		t.Error("<title> übernimmt den konfigurierten Titel nicht")
	}
	if !strings.Contains(html, `class="brand-name">TeamDrop<`) {
		t.Error("Kopfzeile übernimmt den konfigurierten Titel nicht")
	}
	if !strings.Contains(html, `class="header-logo" src="https://static.example.com/logo.png"`) {
		t.Error("konfiguriertes Logo fehlt in der Kopfzeile")
	}
	if !strings.Contains(html, "TeamDrop ist quelloffen") {
		t.Error("Footer übernimmt den konfigurierten Titel nicht")
	}

	var manifest manifestDoc
	if err := json.Unmarshal(assets["manifest.json"].body, &manifest); err != nil {
		t.Fatalf("manifest.json ist kein gültiges JSON: %v", err)
	}
	if manifest.Name != "TeamDrop" || manifest.ShortName != "TeamDrop" {
		t.Errorf("Manifest-Name = %q/%q, erwartet \"TeamDrop\"", manifest.Name, manifest.ShortName)
	}
}

// TestApplyBrandingIsRepeatable hält die eigentliche Falle fest: eine erste
// Fassung las die Vorlage aus der schon gerenderten assets-Map statt aus dem
// unveränderlichen webFS. Ein zweiter Aufruf traf dann auf HTML ohne
// Platzhalter und konnte nichts mehr ersetzen.
func TestApplyBrandingIsRepeatable(t *testing.T) {
	if err := applyBranding(&config.Config{Title: "ZuerstGewaehlt"}); err != nil {
		t.Fatalf("applyBranding (1): %v", err)
	}
	if err := applyBranding(&config.Config{Title: "DanachGewaehlt"}); err != nil {
		t.Fatalf("applyBranding (2): %v", err)
	}

	html := string(assets["index.html"].body)
	if strings.Contains(html, "ZuerstGewaehlt") {
		t.Error("der zweite Durchlauf hat den ersten Titel nicht ersetzt")
	}
	if !strings.Contains(html, "<title>DanachGewaehlt</title>") {
		t.Error("der zweite Durchlauf hat seinen eigenen Titel nicht gesetzt")
	}

	var manifest manifestDoc
	if err := json.Unmarshal(assets["manifest.json"].body, &manifest); err != nil {
		t.Fatalf("manifest.json ist kein gültiges JSON: %v", err)
	}
	if manifest.Name != "DanachGewaehlt" {
		t.Errorf("auch das Manifest hängt noch am ersten Titel: %q", manifest.Name)
	}
}

func TestApplyBrandingEscapesTitle(t *testing.T) {
	if err := applyBranding(&config.Config{Title: `A & <script>alert(1)</script>`}); err != nil {
		t.Fatalf("applyBranding: %v", err)
	}
	html := string(assets["index.html"].body)
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatal("Titel wurde ungeschützt ins HTML übernommen")
	}
	if !strings.Contains(html, "A &amp; &lt;script&gt;") {
		t.Errorf("Titel wurde nicht wie erwartet escaped, HTML enthält: %s",
			html[strings.Index(html, "<title>"):strings.Index(html, "</title>")+8])
	}
}
