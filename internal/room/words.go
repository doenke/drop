package room

import (
	"crypto/rand"
	_ "embed"
	"fmt"
	"math/big"
	"strings"
)

//go:embed words_en.txt
var wordsEnFile string

//go:embed words_de.txt
var wordsDeFile string

// WordsPerCode ist die Länge des menschlichen Beitrittscodes.
const WordsPerCode = 3

// defaultLang ist die Sprache, auf die eine unbekannte oder leere Angabe
// fällt — Englisch ist der neue Standard der App.
const defaultLang = "en"

// wordsByLang sind die kuratierten Wortlisten je Sprache.
var wordsByLang = map[string][]string{
	"en": parseWords(wordsEnFile),
	"de": parseWords(wordsDeFile),
}

func parseWords(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if len(out) < 100 {
		panic("Wortliste ist zu klein")
	}
	return out
}

// resolveLang bildet eine beliebige Client-Angabe auf eine unterstützte
// Sprache ab; unbekannt oder leer landet auf defaultLang.
func resolveLang(lang string) string {
	if _, ok := wordsByLang[lang]; ok {
		return lang
	}
	return defaultLang
}

// WordCount gibt die Größe der Wortliste einer Sprache zurück (für Tests und
// Logging).
func WordCount(lang string) int { return len(wordsByLang[resolveLang(lang)]) }

// newWordCode zieht WordsPerCode Wörter aus der Liste der angegebenen
// Sprache mit crypto/rand. Wiederholungen sind erlaubt — sie kosten kaum
// Entropie und die Prüfung wäre nur Ballast.
func newWordCode(lang string) (string, error) {
	words := wordsByLang[resolveLang(lang)]
	max := big.NewInt(int64(len(words)))
	parts := make([]string, WordsPerCode)
	for i := range parts {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("Zufallswort: %w", err)
		}
		parts[i] = words[n.Int64()]
	}
	return strings.Join(parts, "-"), nil
}

// NormalizeCode macht Tippvarianten vergleichbar: Groß-/Kleinschreibung,
// Leerzeichen statt Bindestrich, Umlaute im Eingabefeld. Damit funktioniert
// sowohl "Apfel Tanne Kerze" als auch "apfel-tanne-kerze".
func NormalizeCode(in string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	replacer := strings.NewReplacer(
		"ä", "a", "ö", "o", "ü", "u", "ß", "ss",
		",", " ", ".", " ", "-", " ", "_", " ", "\t", " ",
	)
	fields := strings.Fields(replacer.Replace(in))
	return strings.Join(fields, "-")
}

// deviceLabels sind die Präfixe für die generischen Gerätenamen ("Device 1",
// "Gerät 1"), je nach Raumsprache.
var deviceLabels = map[string]string{
	"en": "Device ",
	"de": "Gerät ",
}

func deviceLabel(lang string, ord int) string {
	return deviceLabels[resolveLang(lang)] + fmt.Sprint(ord)
}
