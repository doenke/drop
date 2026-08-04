package room

import (
	"regexp"
	"strings"
	"testing"
)

var wordPattern = regexp.MustCompile(`^[a-z]{4,12}$`)

// TestWordListInvariants hält die in SPEC.md geforderten Eigenschaften jeder
// Liste fest: kleingeschrieben, keine Umlaute, tippfreundlich, eindeutig.
func TestWordListInvariants(t *testing.T) {
	for lang, words := range wordsByLang {
		t.Run(lang, func(t *testing.T) {
			if len(words) < 400 {
				t.Fatalf("Wortliste zu klein: %d", len(words))
			}
			seen := map[string]bool{}
			for _, w := range words {
				if !wordPattern.MatchString(w) {
					t.Errorf("Wort verstößt gegen das Format: %q", w)
				}
				if seen[w] {
					t.Errorf("doppeltes Wort: %q", w)
				}
				seen[w] = true
			}
		})
	}
}

// TestWordsAreTypoDistant stellt sicher, dass innerhalb jeder Liste keine
// zwei Wörter nur einen Tippfehler auseinanderliegen — sonst führt ein
// Vertipper auf einen fremden Raum statt auf eine Fehlermeldung.
func TestWordsAreTypoDistant(t *testing.T) {
	for lang, words := range wordsByLang {
		t.Run(lang, func(t *testing.T) {
			for i := 0; i < len(words); i++ {
				for j := i + 1; j < len(words); j++ {
					if editDistanceAtMostOne(words[i], words[j]) {
						t.Errorf("zu ähnlich: %q und %q", words[i], words[j])
					}
				}
			}
		})
	}
}

func TestNewWordCode(t *testing.T) {
	for lang := range wordsByLang {
		t.Run(lang, func(t *testing.T) {
			seen := map[string]bool{}
			for i := 0; i < 200; i++ {
				code, err := newWordCode(lang)
				if err != nil {
					t.Fatalf("newWordCode: %v", err)
				}
				parts := strings.Split(code, "-")
				if len(parts) != WordsPerCode {
					t.Fatalf("Code hat %d Teile: %q", len(parts), code)
				}
				for _, p := range parts {
					if !wordPattern.MatchString(p) {
						t.Fatalf("Codeteil passt nicht zum Format: %q", p)
					}
				}
				seen[code] = true
			}
			// Bei mehreren hundert Wörtern hoch drei wären Dubletten in 200
			// Ziehungen ein Zeichen für einen kaputten Zufallsgenerator.
			if len(seen) < 200 {
				t.Fatalf("nur %d verschiedene Codes aus 200 Ziehungen", len(seen))
			}
		})
	}
}

func TestNewWordCodeFallsBackToDefaultLang(t *testing.T) {
	code, err := newWordCode("xx")
	if err != nil {
		t.Fatalf("newWordCode: %v", err)
	}
	for _, p := range strings.Split(code, "-") {
		found := false
		for _, w := range wordsByLang[defaultLang] {
			if w == p {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Codeteil %q stammt nicht aus der Standardliste %q", p, defaultLang)
		}
	}
}

func TestNormalizeCode(t *testing.T) {
	cases := map[string]string{
		"apfel-tanne-kerze":     "apfel-tanne-kerze",
		"Apfel Tanne Kerze":     "apfel-tanne-kerze",
		"  apfel  tanne kerze ": "apfel-tanne-kerze",
		"APFEL_TANNE_KERZE":     "apfel-tanne-kerze",
		"apfel, tanne, kerze":   "apfel-tanne-kerze",
		"Apfel-Tanne-Kerze":     "apfel-tanne-kerze",
		"":                      "",
	}
	for in, want := range cases {
		if got := NormalizeCode(in); got != want {
			t.Errorf("NormalizeCode(%q) = %q, erwartet %q", in, got, want)
		}
	}
}

// editDistanceAtMostOne prüft auf Levenshtein-Distanz ≤ 1 inklusive
// Vertauschung benachbarter Zeichen (optimal string alignment).
func editDistanceAtMostOne(a, b string) bool {
	if a == b {
		return true
	}
	if len(a)-len(b) > 1 || len(b)-len(a) > 1 {
		return false
	}
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, min(d[i][j-1]+1, d[i-1][j-1]+cost))
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)] <= 1
}
