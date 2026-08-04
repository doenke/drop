package ws

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/doenke/drop/internal/auth"
	"github.com/doenke/drop/internal/httpx"
	"github.com/doenke/drop/internal/room"
)

type testEnv struct {
	server  *httptest.Server
	signer  *auth.Signer
	hub     *room.Hub
	limiter *httpx.Limiter
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	signer := auth.NewSigner([]byte("0123456789abcdef0123456789abcdef"), time.Hour, false)
	hub := room.NewHub(room.Options{EmptyGrace: time.Minute, MaxAge: time.Hour, MaxMembers: 3})
	limiter := httpx.NewLimiter(600, 50)
	h := New(hub, signer, limiter, Limits{
		MaxFileSize:     1 << 20,
		MaxChunkSize:    64 << 10,
		MaxTextItemSize: 4 << 10,
		MaxLiveTextSize: 4 << 10,
	}, "", func(token string) string { return "https://drop.example/r/" + token },
		false, slog.New(slog.DiscardHandler))

	mux := http.NewServeMux()
	mux.Handle("GET /ws", h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &testEnv{server: srv, signer: signer, hub: hub, limiter: limiter}
}

// client ist eine Testverbindung mit den Bequemlichkeiten, die alle Fälle
// brauchen: senden, auf eine bestimmte Nachricht warten, Binärframes lesen.
type client struct {
	t    *testing.T
	conn *websocket.Conn
	ctx  context.Context
}

// dial öffnet eine Verbindung; mit loggedIn wird ein gültiges Session-Cookie
// mitgeschickt, ohne bleibt es ein Gast.
func (e *testEnv) dial(t *testing.T, loggedIn bool) *client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	header := http.Header{}
	if loggedIn {
		rec := httptest.NewRecorder()
		if err := e.signer.Issue(rec, auth.Session{Subject: "user-1", Name: "Test"}); err != nil {
			t.Fatalf("Session ausstellen: %v", err)
		}
		header.Set("Cookie", rec.Result().Cookies()[0].String())
	}

	conn, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.server.URL, "http")+"/ws",
		&websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Dial: %v (%s)", err, body)
		}
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return &client{t: t, conn: conn, ctx: ctx}
}

func (c *client) send(v any) {
	c.t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		c.t.Fatalf("Marshal: %v", err)
	}
	if err := c.conn.Write(c.ctx, websocket.MessageText, data); err != nil {
		c.t.Fatalf("Write: %v", err)
	}
}

func (c *client) sendBinary(data []byte) {
	c.t.Helper()
	if err := c.conn.Write(c.ctx, websocket.MessageBinary, data); err != nil {
		c.t.Fatalf("Write binär: %v", err)
	}
}

func (c *client) read() (websocket.MessageType, []byte) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	typ, data, err := c.conn.Read(ctx)
	if err != nil {
		c.t.Fatalf("Read: %v", err)
	}
	return typ, data
}

// expect wartet auf die nächste Nachricht des angegebenen Typs. Presence-
// Nachrichten dazwischen werden übersprungen, damit die Tests nicht an der
// Reihenfolge hängen.
func (c *client) expect(msgType string) map[string]any {
	c.t.Helper()
	for i := 0; i < 10; i++ {
		typ, data := c.read()
		if typ != websocket.MessageText {
			c.t.Fatalf("erwartet Textframe %q, bekommen Binärframe", msgType)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			c.t.Fatalf("Unmarshal: %v", err)
		}
		if m["type"] == msgType {
			return m
		}
		if m["type"] == msgError {
			c.t.Fatalf("erwartet %q, bekommen Fehler %v: %v", msgType, m["code"], m["message"])
		}
	}
	c.t.Fatalf("Nachricht %q kam nicht", msgType)
	return nil
}

// createRoom legt als angemeldeter Nutzer einen Raum an.
func (e *testEnv) createRoom(t *testing.T) (*client, map[string]any) {
	t.Helper()
	c := e.dial(t, true)
	c.send(map[string]any{"type": msgCreate})
	return c, c.expect(msgRoom)
}

func TestCreateRequiresSession(t *testing.T) {
	e := newEnv(t)
	guest := e.dial(t, false)
	guest.send(map[string]any{"type": msgCreate})

	m := guest.expect(msgError)
	if m["code"] != errUnauthorized {
		t.Fatalf("Fehlercode = %v, erwartet %q", m["code"], errUnauthorized)
	}
	if e.hub.Count() != 0 {
		t.Fatalf("Gast konnte einen Raum anlegen")
	}
}

func TestCreateAndJoinByCode(t *testing.T) {
	e := newEnv(t)
	owner, created := e.createRoom(t)

	if created["created"] != true {
		t.Fatalf("created-Flag fehlt: %v", created)
	}
	code, _ := created["code"].(string)
	if strings.Count(code, "-") != room.WordsPerCode-1 {
		t.Fatalf("Code sieht nicht nach drei Wörtern aus: %q", code)
	}
	if url, _ := created["url"].(string); !strings.HasSuffix(url, created["token"].(string)) {
		t.Fatalf("Raum-URL trägt nicht den Token: %q", url)
	}

	// Ein Gast tritt ohne Login bei — in der Schreibweise, die er abtippt.
	guest := e.dial(t, false)
	guest.send(map[string]any{"type": msgJoin, "code": strings.ToUpper(strings.ReplaceAll(code, "-", " "))})
	joined := guest.expect(msgRoom)
	if joined["id"] != created["id"] {
		t.Fatalf("Gast landete in Raum %v statt %v", joined["id"], created["id"])
	}
	if joined["created"] == true {
		t.Fatalf("Beitritt wurde als Neuanlage gemeldet")
	}

	// Der Ersteller erfährt vom Beitritt.
	peerJoined := owner.expect(msgPeerJoined)
	if peerJoined["members"] != float64(2) {
		t.Fatalf("Teilnehmerzahl = %v, erwartet 2", peerJoined["members"])
	}
}

func TestJoinByToken(t *testing.T) {
	e := newEnv(t)
	_, created := e.createRoom(t)

	guest := e.dial(t, false)
	guest.send(map[string]any{"type": msgJoin, "token": created["token"]})
	if joined := guest.expect(msgRoom); joined["id"] != created["id"] {
		t.Fatalf("Token führte in den falschen Raum")
	}
}

func TestJoinUnknownCode(t *testing.T) {
	e := newEnv(t)
	guest := e.dial(t, false)
	guest.send(map[string]any{"type": msgJoin, "code": "gibt-es-nicht"})

	if m := guest.expect(msgError); m["code"] != errNotFound {
		t.Fatalf("Fehlercode = %v, erwartet %q", m["code"], errNotFound)
	}
}

func TestJoinIsRateLimited(t *testing.T) {
	e := newEnv(t)
	// Ein sehr enges Budget, damit der Schutz im Test greift.
	e.limiter = httpx.NewLimiter(1, 2)
	h := New(e.hub, e.signer, e.limiter, Limits{MaxFileSize: 1 << 20, MaxChunkSize: 64 << 10,
		MaxTextItemSize: 4 << 10, MaxLiveTextSize: 4 << 10}, "",
		func(string) string { return "" }, false, slog.New(slog.DiscardHandler))
	mux := http.NewServeMux()
	mux.Handle("GET /ws", h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	e.server = srv

	for i := 0; i < 2; i++ {
		c := e.dial(t, false)
		c.send(map[string]any{"type": msgJoin, "code": "gibt-es-nicht"})
		if m := c.expect(msgError); m["code"] != errNotFound {
			t.Fatalf("Versuch %d: %v", i+1, m["code"])
		}
	}
	c := e.dial(t, false)
	c.send(map[string]any{"type": msgJoin, "code": "gibt-es-nicht"})
	if m := c.expect(msgError); m["code"] != errRateLimited {
		t.Fatalf("drittes Durchprobieren wurde nicht gebremst: %v", m["code"])
	}
}

func TestTextSyncIsMirroredAndSnapshotted(t *testing.T) {
	e := newEnv(t)
	owner, created := e.createRoom(t)

	guest := e.dial(t, false)
	guest.send(map[string]any{"type": msgJoin, "code": created["code"]})
	guest.expect(msgRoom)
	owner.expect(msgPeerJoined)

	owner.send(map[string]any{"type": msgTextSync, "full": "hallo welt"})
	mirrored := guest.expect(msgTextSync)
	if mirrored["full"] != "hallo welt" {
		t.Fatalf("Text kam nicht an: %v", mirrored["full"])
	}
	if seq, _ := mirrored["seq"].(float64); seq < 1 {
		t.Fatalf("Sequenznummer fehlt: %v", mirrored["seq"])
	}

	// Wer später kommt, sieht den Stand der Box sofort.
	latecomer := e.dial(t, false)
	latecomer.send(map[string]any{"type": msgJoin, "code": created["code"]})
	if snapshot := latecomer.expect(msgRoom); snapshot["text"] != "hallo welt" {
		t.Fatalf("Nachzügler bekam keinen Textstand: %v", snapshot["text"])
	}
}

func TestTextSyncRejectsOversizedText(t *testing.T) {
	e := newEnv(t)
	owner, _ := e.createRoom(t)

	owner.send(map[string]any{"type": msgTextSync, "full": strings.Repeat("x", 5<<10)})
	if m := owner.expect(msgError); m["code"] != errLiveTextTooLarge {
		t.Fatalf("zu langer Text wurde angenommen: %v", m["code"])
	}
}

func TestItemTextReachesOtherMembersOnly(t *testing.T) {
	e := newEnv(t)
	owner, created := e.createRoom(t)

	guest := e.dial(t, false)
	guest.send(map[string]any{"type": msgJoin, "code": created["code"]})
	guest.expect(msgRoom)
	owner.expect(msgPeerJoined)

	owner.send(map[string]any{"type": msgItemText, "content": "geheimes-passwort"})
	item := guest.expect(msgItemText)
	if item["content"] != "geheimes-passwort" {
		t.Fatalf("Snippet kam nicht an: %v", item["content"])
	}
	if from, _ := item["from"].(map[string]any); from["id"] == nil {
		t.Fatalf("Absender fehlt: %v", item["from"])
	}

	// Der Absender bekommt sein eigenes Snippet nicht zurück — die Anzeige
	// macht das Frontend lokal.
	owner.send(map[string]any{"type": msgTextSync, "full": "marker"})
	if next := guest.expect(msgTextSync); next["full"] != "marker" {
		t.Fatalf("unerwartete Nachricht nach dem Snippet")
	}
}

func TestFileRoundtrip(t *testing.T) {
	e := newEnv(t)
	owner, created := e.createRoom(t)

	guest := e.dial(t, false)
	guest.send(map[string]any{"type": msgJoin, "code": created["code"]})
	guest.expect(msgRoom)
	owner.expect(msgPeerJoined)

	payload := bytes.Repeat([]byte("drop"), 4096) // 16 KiB
	const fileID = "f1"

	owner.send(map[string]any{
		"type": msgFileMeta, "id": fileID,
		"name": "../../etc/passwd", "mime": "text/plain", "size": len(payload),
	})
	meta := guest.expect(msgFileMeta)
	// Der Server entschärft den Namen, bevor er beim Empfänger als
	// Download-Name landet.
	if meta["name"] != "passwd" {
		t.Fatalf("Dateiname wurde nicht entschärft: %v", meta["name"])
	}
	if meta["size"] != float64(len(payload)) {
		t.Fatalf("Größe = %v", meta["size"])
	}

	owner.sendBinary(binaryFrame(fileID, payload[:8192]))
	owner.sendBinary(binaryFrame(fileID, payload[8192:]))

	var got []byte
	for len(got) < len(payload) {
		typ, data := guest.read()
		if typ != websocket.MessageBinary {
			t.Fatalf("erwartet Binärframe, bekommen: %s", data)
		}
		id, chunk, err := splitBinaryFrame(data)
		if err != nil || id != fileID {
			t.Fatalf("Binärframe unbrauchbar: id=%q err=%v", id, err)
		}
		got = append(got, chunk...)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("Inhalt kam verändert an (%d von %d Bytes)", len(got), len(payload))
	}

	owner.send(map[string]any{"type": msgFileEnd, "id": fileID})
	if end := guest.expect(msgFileEnd); end["id"] != fileID {
		t.Fatalf("file-end trägt die falsche ID: %v", end["id"])
	}
}

func TestFileChunkWithoutMetaIsRejected(t *testing.T) {
	e := newEnv(t)
	owner, _ := e.createRoom(t)

	owner.sendBinary(binaryFrame("unbekannt", []byte("daten")))
	if m := owner.expect(msgError); m["code"] != errChunkUnannounced {
		t.Fatalf("Chunk ohne Ankündigung wurde angenommen: %v", m["code"])
	}
}

func TestFileLargerThanAnnouncedIsAborted(t *testing.T) {
	e := newEnv(t)
	owner, created := e.createRoom(t)

	guest := e.dial(t, false)
	guest.send(map[string]any{"type": msgJoin, "code": created["code"]})
	guest.expect(msgRoom)
	owner.expect(msgPeerJoined)

	owner.send(map[string]any{"type": msgFileMeta, "id": "f1", "name": "a.bin", "mime": "", "size": 4})
	guest.expect(msgFileMeta)

	owner.sendBinary(binaryFrame("f1", []byte("viel zu viele Bytes")))
	if m := owner.expect(msgError); m["code"] != errUploadOverflow {
		t.Fatalf("Überlänge wurde nicht bemerkt: %v", m["code"])
	}
	if abort := guest.expect(msgFileAbort); abort["id"] != "f1" {
		t.Fatalf("Empfänger wurde nicht über den Abbruch informiert")
	}
}

func TestOversizedFileIsRefusedUpfront(t *testing.T) {
	e := newEnv(t)
	owner, _ := e.createRoom(t)

	owner.send(map[string]any{"type": msgFileMeta, "id": "f1", "name": "gross.bin", "size": 2 << 20})
	if m := owner.expect(msgError); m["code"] != errFileTooLarge {
		t.Fatalf("zu große Datei wurde angekündigt: %v", m["code"])
	}
}

func TestIncompleteFileEndsAsAbort(t *testing.T) {
	e := newEnv(t)
	owner, created := e.createRoom(t)

	guest := e.dial(t, false)
	guest.send(map[string]any{"type": msgJoin, "code": created["code"]})
	guest.expect(msgRoom)
	owner.expect(msgPeerJoined)

	owner.send(map[string]any{"type": msgFileMeta, "id": "f1", "name": "a.bin", "size": 100})
	guest.expect(msgFileMeta)
	owner.sendBinary(binaryFrame("f1", []byte("nur ein Anfang")))
	guest.read() // der Chunk

	owner.send(map[string]any{"type": msgFileEnd, "id": "f1"})
	if abort := guest.expect(msgFileAbort); abort["id"] != "f1" {
		t.Fatalf("unvollständige Datei wurde als fertig gemeldet")
	}
}

func TestPeerLeftOnDisconnect(t *testing.T) {
	e := newEnv(t)
	owner, created := e.createRoom(t)

	guest := e.dial(t, false)
	guest.send(map[string]any{"type": msgJoin, "code": created["code"]})
	guest.expect(msgRoom)
	owner.expect(msgPeerJoined)

	_ = guest.conn.Close(websocket.StatusNormalClosure, "tschuess")
	if left := owner.expect(msgPeerLeft); left["members"] != float64(1) {
		t.Fatalf("Teilnehmerzahl nach dem Abgang = %v", left["members"])
	}
}

func TestRoomFull(t *testing.T) {
	e := newEnv(t) // MaxMembers ist 3
	_, created := e.createRoom(t)

	for i := 0; i < 2; i++ {
		c := e.dial(t, false)
		c.send(map[string]any{"type": msgJoin, "code": created["code"]})
		c.expect(msgRoom)
	}
	overflow := e.dial(t, false)
	overflow.send(map[string]any{"type": msgJoin, "code": created["code"]})
	if m := overflow.expect(msgError); m["code"] != errRoomFull {
		t.Fatalf("vierter Teilnehmer wurde eingelassen: %v", m["code"])
	}
}

func TestActionsRequireRoom(t *testing.T) {
	e := newEnv(t)
	c := e.dial(t, false)
	c.send(map[string]any{"type": msgTextSync, "full": "ohne Raum"})
	if m := c.expect(msgError); m["code"] != errNotInRoom {
		t.Fatalf("Aktion ohne Raum wurde angenommen: %v", m["code"])
	}
}

func TestUnknownMessageType(t *testing.T) {
	e := newEnv(t)
	c := e.dial(t, false)
	c.send(map[string]any{"type": "gibt-es-nicht"})
	if m := c.expect(msgError); m["code"] != errUnknownType {
		t.Fatalf("unbekannter Typ wurde akzeptiert: %v", m["code"])
	}
}

func binaryFrame(id string, payload []byte) []byte {
	out := make([]byte, 2+len(id)+len(payload))
	binary.BigEndian.PutUint16(out[:2], uint16(len(id)))
	copy(out[2:], id)
	copy(out[2+len(id):], payload)
	return out
}
