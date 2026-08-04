// Package ws implementiert das WebSocket-Protokoll: ein Endpoint, über den
// Räume angelegt, betreten und alle Inhalte relayt werden.
package ws

import (
	"encoding/binary"
	"encoding/json"
	"errors"
)

// Nachrichtentypen Client → Server.
const (
	msgCreate   = "create"
	msgJoin     = "join"
	msgTextSync = "text-sync"
	msgItemText = "item-text"
	msgFileMeta = "file-meta"
	msgFileEnd  = "file-end"
)

// Nachrichtentypen Server → Client. text-sync, item-text, file-meta und
// file-end werden gespiegelt und tauchen deshalb in beiden Richtungen auf.
const (
	msgRoom       = "room"
	msgPeerJoined = "peer-joined"
	msgPeerLeft   = "peer-left"
	msgFileAbort  = "file-abort"
	msgError      = "error"
)

// Fehlercodes, die das Frontend unterscheiden kann. Ein Code pro
// unterschiedlicher Situation, damit beim Übersetzen keine Nuance verloren
// geht — echte Dubletten (z. B. mehrere "erst einem Raum beitreten"-Stellen)
// teilen sich weiterhin einen Code.
const (
	errUnauthorized       = "unauthorized"
	errNotFound           = "room-not-found"
	errRoomFull           = "room-full"
	errRateLimited        = "rate-limited"
	errInvalidJSON        = "invalid-json"
	errUnknownType        = "unknown-type"
	errAlreadyInRoom      = "already-in-room"
	errCreateFailed       = "create-failed"
	errMissingCodeOrToken = "missing-code-or-token"
	errNotInRoom          = "not-in-room"
	errTextSyncEmpty      = "text-sync-empty"
	errLiveTextTooLarge   = "live-text-too-large"
	errItemTextEmpty      = "item-text-empty"
	errTextItemTooLarge   = "text-item-too-large"
	errFileIDInvalid      = "file-id-invalid"
	errUploadDuplicateID  = "upload-duplicate-id"
	errTooManyUploads     = "too-many-uploads"
	errFileTooLarge       = "file-too-large"
	errBinaryFrameInvalid = "binary-frame-invalid"
	errChunkUnannounced   = "chunk-unannounced"
	errChunkTooLarge      = "chunk-too-large"
	errUploadOverflow     = "upload-overflow"
	errFileEndUnknown     = "file-end-unknown"
)

// clientMsg deckt alle eingehenden Steuernachrichten ab. Full ist ein Zeiger,
// damit sich ein geleertes Textfeld ("") von einer fehlenden Angabe
// unterscheiden lässt.
type clientMsg struct {
	Type    string  `json:"type"`
	Code    string  `json:"code,omitempty"`
	Token   string  `json:"token,omitempty"`
	Full    *string `json:"full,omitempty"`
	Content string  `json:"content,omitempty"`
	ID      string  `json:"id,omitempty"`
	Name    string  `json:"name,omitempty"`
	Mime    string  `json:"mime,omitempty"`
	Size    int64   `json:"size,omitempty"`
	Lang    string  `json:"lang,omitempty"`
}

// peer ist die abgespeckte Sicht auf ein Mitglied, die andere sehen dürfen.
type peer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner bool   `json:"owner,omitempty"`
}

type roomMsg struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Token   string `json:"token"`
	Code    string `json:"code"`
	URL     string `json:"url"`
	You     peer   `json:"you"`
	Peers   []peer `json:"peers"`
	Created bool   `json:"created"`
	Text    string `json:"text,omitempty"`
	TextSeq uint64 `json:"textSeq"`
}

type peerMsg struct {
	Type    string `json:"type"`
	Peer    peer   `json:"peer"`
	Members int    `json:"members"`
}

type textSyncMsg struct {
	Type string `json:"type"`
	Full string `json:"full"`
	Seq  uint64 `json:"seq"`
	From string `json:"from"`
}

type itemTextMsg struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Content string `json:"content"`
	From    peer   `json:"from"`
}

type fileMetaMsg struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
	From peer   `json:"from"`
}

type fileIDMsg struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type errorMsg struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func encode(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		// Alle Nachrichten sind einfache Structs; ein Fehler wäre ein Bug.
		panic("Nachricht nicht serialisierbar: " + err.Error())
	}
	return b
}

var errBadFrame = errors.New("Binärframe ist fehlerhaft")

// Binärframes tragen die Datei-ID im Kopf, damit mehrere Übertragungen
// parallel laufen können, ohne dass der Empfänger raten muss:
//
//	uint16 (big endian) Länge der ID | ID (UTF-8) | Rohdaten
const maxBinaryIDLen = 64

func splitBinaryFrame(frame []byte) (id string, payload []byte, err error) {
	if len(frame) < 2 {
		return "", nil, errBadFrame
	}
	n := int(binary.BigEndian.Uint16(frame[:2]))
	if n == 0 || n > maxBinaryIDLen || len(frame) < 2+n {
		return "", nil, errBadFrame
	}
	return string(frame[2 : 2+n]), frame[2+n:], nil
}
