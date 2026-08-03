package room

import (
	"crypto/rand"
	_ "embed"
	"fmt"
	"math/big"
	"strings"
)

//go:embed words.txt
var wordsFile string

// WordsPerCode ist die Länge des menschlichen Beitrittscodes.
const WordsPerCode = 3

// words ist die kuratierte Wortliste aus words.txt.
var words = parseWords(wordsFile)

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

// WordCount gibt die Größe der Wortliste zurück (für Tests und Logging).
func WordCount() int { return len(words) }

// newWordCode zieht WordsPerCode Wörter mit crypto/rand. Wiederholungen sind
// erlaubt — sie kosten kaum Entropie und die Prüfung wäre nur Ballast.
func newWordCode() (string, error) {
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
