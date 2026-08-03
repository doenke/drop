package ws

import (
	"path"
	"strings"
	"unicode"

	"github.com/doenke/drop/internal/room"
)

// maxOpenUploads begrenzt, wie viele Dateien eine Verbindung gleichzeitig
// hochladen darf. Das Frontend schickt seriell; die Grenze fängt nur ab, dass
// jemand den Server mit angekündigten Transfers zumüllt.
const maxOpenUploads = 8

// upload merkt sich einen angekündigten Transfer, damit der Server die
// Binärframes zuordnen und das Größenbudget mitzählen kann. Der Inhalt selbst
// wird nie zwischengespeichert — er geht direkt weiter.
type upload struct {
	size int64
	sent int64
}

func (c *conn) handleFileMeta(msg clientMsg) error {
	if c.member == nil {
		c.fail(errRoomState, "Erst einem Raum beitreten")
		return nil
	}
	id := msg.ID
	if id == "" || len(id) > maxBinaryIDLen {
		c.fail(errBadMessage, "file-meta ohne brauchbare ID")
		return nil
	}
	if _, exists := c.uploads[id]; exists {
		c.fail(errBadMessage, "Diese Übertragung läuft bereits")
		return nil
	}
	if len(c.uploads) >= maxOpenUploads {
		c.fail(errRoomState, "Zu viele gleichzeitige Übertragungen")
		return nil
	}
	if msg.Size <= 0 || msg.Size > c.h.limits.MaxFileSize {
		c.fail(errTooLarge, "Die Datei ist zu groß")
		return nil
	}

	name := sanitizeFilename(msg.Name)
	mime := sanitizeMime(msg.Mime)
	c.uploads[id] = &upload{size: msg.Size}

	c.room.Broadcast(c.member, frame(fileMetaMsg{
		Type: msgFileMeta,
		ID:   id,
		Name: name,
		Mime: mime,
		Size: msg.Size,
		From: asPeer(c.member),
	}))
	return nil
}

// handleBinary reicht einen Datei-Chunk weiter. Der Frame wird unverändert
// gespiegelt — der Server kopiert nichts auf Platte und hält nichts fest.
func (c *conn) handleBinary(data []byte) error {
	if c.member == nil {
		c.fail(errRoomState, "Erst einem Raum beitreten")
		return nil
	}
	id, payload, err := splitBinaryFrame(data)
	if err != nil {
		c.fail(errBadMessage, "Binärframe ist fehlerhaft")
		return nil
	}
	up, ok := c.uploads[id]
	if !ok {
		c.fail(errBadMessage, "Chunk ohne angekündigte Datei")
		return nil
	}
	if int64(len(payload)) > c.h.limits.MaxChunkSize {
		c.abortUpload(id, "Chunk ist zu groß")
		return nil
	}
	if up.sent+int64(len(payload)) > up.size {
		c.abortUpload(id, "Es kamen mehr Daten als angekündigt")
		return nil
	}
	up.sent += int64(len(payload))

	c.room.Broadcast(c.member, room.Frame{Binary: true, Data: data})
	return nil
}

func (c *conn) handleFileEnd(msg clientMsg) error {
	if c.member == nil {
		c.fail(errRoomState, "Erst einem Raum beitreten")
		return nil
	}
	up, ok := c.uploads[msg.ID]
	if !ok {
		c.fail(errBadMessage, "file-end ohne laufende Übertragung")
		return nil
	}
	delete(c.uploads, msg.ID)

	// Eine unvollständige Datei ist beim Empfänger wertlos; besser abbrechen
	// als etwas Kaputtes zum Download anbieten.
	if up.sent != up.size {
		c.room.Broadcast(c.member, frame(fileIDMsg{Type: msgFileAbort, ID: msg.ID}))
		return nil
	}
	c.room.Broadcast(c.member, frame(fileIDMsg{Type: msgFileEnd, ID: msg.ID}))
	return nil
}

// abortUpload bricht eine laufende Übertragung ab und sagt es beiden Seiten.
func (c *conn) abortUpload(id, reason string) {
	delete(c.uploads, id)
	c.room.Broadcast(c.member, frame(fileIDMsg{Type: msgFileAbort, ID: id}))
	c.fail(errTooLarge, reason)
}

// abortUploads räumt beim Verbindungsabbruch auf, damit die Gegenstellen ihre
// halben Dateien verwerfen können.
func (c *conn) abortUploads() {
	for id := range c.uploads {
		delete(c.uploads, id)
		c.room.Broadcast(c.member, frame(fileIDMsg{Type: msgFileAbort, ID: id}))
	}
}

// sanitizeFilename macht aus dem gemeldeten Namen etwas, das der Empfänger
// gefahrlos als Download-Namen verwenden kann.
func sanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		if r == '\\' {
			return '/'
		}
		return r
	}, name)
	// Erst den letzten Pfadbestandteil ziehen, dann prüfen: sonst würde aus
	// "../../etc/passwd" durch reines Zeichenlöschen "....etcpasswd".
	name = strings.TrimSpace(path.Base(strings.TrimSpace(name)))
	if name == "." || name == ".." || name == "/" || name == "" {
		return "datei"
	}
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}

func sanitizeMime(mime string) string {
	mime = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, mime)
	if len(mime) > 128 {
		return ""
	}
	return mime
}
