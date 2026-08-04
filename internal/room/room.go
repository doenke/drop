// Package room verwaltet die Räume. Alles liegt im RAM: es gibt keine
// Datenbank und kein Volume, und ein leerer Raum verschwindet nach kurzer
// Zeit samt seinen Codes.
package room

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrRoomNotFound = errors.New("Raum nicht gefunden")
	ErrRoomFull     = errors.New("Raum ist voll")
	ErrRoomClosed   = errors.New("Raum ist geschlossen")
)

// Frame ist eine ausgehende WebSocket-Nachricht. Steuernachrichten sind
// Textframes (JSON), Dateiinhalte Binärframes.
type Frame struct {
	Binary bool
	Data   []byte
}

// Member ist eine verbundene Gegenstelle. Der Schreib-Puffer ist bewusst
// klein: wer nicht hinterherkommt, wird getrennt, statt den Serverspeicher
// mit Dateichunks vollaufen zu lassen.
type Member struct {
	ID    string
	Name  string
	Owner bool

	out       chan Frame
	closed    chan struct{}
	closeOnce sync.Once
	slow      atomic.Bool
}

const sendBuffer = 32

func newMember(id, name string, owner bool) *Member {
	return &Member{
		ID:     id,
		Name:   name,
		Owner:  owner,
		out:    make(chan Frame, sendBuffer),
		closed: make(chan struct{}),
	}
}

// Out liefert die Frames, die der Schreiber-Goroutine rausschicken soll.
func (m *Member) Out() <-chan Frame { return m.out }

// Closed schließt, sobald der Member getrennt werden soll.
func (m *Member) Closed() <-chan struct{} { return m.closed }

// Send stellt einen Frame in den Puffer. Ist der voll, gilt der Member als
// zu langsam und wird geschlossen; false signalisiert das dem Aufrufer.
func (m *Member) Send(f Frame) bool {
	select {
	case <-m.closed:
		return false
	case m.out <- f:
		return true
	default:
		m.slow.Store(true)
		m.Close()
		return false
	}
}

// Close ist idempotent und weckt alle Wartenden.
func (m *Member) Close() {
	m.closeOnce.Do(func() { close(m.closed) })
}

// TooSlow sagt, ob der Member wegen vollem Puffer geschlossen wurde.
func (m *Member) TooSlow() bool { return m.slow.Load() }

// Room ist ein offener Raum mit seinen Mitgliedern und dem letzten Stand der
// Live-Textbox.
type Room struct {
	ID      string
	Token   string // langer Zufallstoken, steckt im QR-Link
	Code    string // 3-Wörter-Code, menschlicher Fallback
	OwnerID string
	Lang    string // Sprache des Erstellers; bestimmt Wortliste und Gerätenamen
	Created time.Time

	maxMembers int
	now        func() time.Time

	mu         sync.Mutex
	members    map[*Member]struct{}
	nextOrd    int
	text       string
	textSeq    uint64
	emptySince time.Time
	closed     bool
}

// Join nimmt einen Member auf und vergibt einen sprechenden Gerätenamen.
func (r *Room) Join(id string, owner bool) (*Member, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrRoomClosed
	}
	if len(r.members) >= r.maxMembers {
		return nil, ErrRoomFull
	}
	r.nextOrd++
	m := newMember(id, deviceLabel(r.Lang, r.nextOrd), owner)
	r.members[m] = struct{}{}
	r.emptySince = time.Time{}
	return m, nil
}

// Leave entfernt einen Member und startet bei Bedarf die Grace-Time.
func (r *Room) Leave(m *Member) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.members[m]; !ok {
		return
	}
	delete(r.members, m)
	m.Close()
	if len(r.members) == 0 {
		r.emptySince = r.now()
	}
}

// Broadcast schickt einen Frame an alle Mitglieder außer dem Absender.
// Übergibt man nil als Absender, bekommen ihn alle.
func (r *Room) Broadcast(from *Member, f Frame) {
	r.mu.Lock()
	targets := make([]*Member, 0, len(r.members))
	for m := range r.members {
		if m != from {
			targets = append(targets, m)
		}
	}
	r.mu.Unlock()

	// Außerhalb des Locks senden: Send kann einen langsamen Member schließen,
	// dessen Leseschleife dann Leave aufruft.
	for _, m := range targets {
		m.Send(f)
	}
}

// Members liefert eine Momentaufnahme der Mitglieder.
func (r *Room) Members() []*Member {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Member, 0, len(r.members))
	for m := range r.members {
		out = append(out, m)
	}
	return out
}

// MemberCount ist die aktuelle Teilnehmerzahl.
func (r *Room) MemberCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.members)
}

// SetText übernimmt einen neuen Stand der Live-Box (Last-Write-Wins) und gibt
// die vom Server vergebene Sequenznummer zurück.
func (r *Room) SetText(text string) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.text = text
	r.textSeq++
	return r.textSeq
}

// Text liefert den aktuellen Stand samt Sequenznummer, damit Nachzügler beim
// Beitritt den Text sehen, der gerade in der Box steht.
func (r *Room) Text() (string, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.text, r.textSeq
}

// close trennt alle Mitglieder; wird vom Hub beim Aufräumen gerufen.
func (r *Room) close() {
	r.mu.Lock()
	r.closed = true
	members := make([]*Member, 0, len(r.members))
	for m := range r.members {
		members = append(members, m)
	}
	r.members = map[*Member]struct{}{}
	r.mu.Unlock()

	for _, m := range members {
		m.Close()
	}
}

// Options steuert Lifecycle und Größe der Räume.
type Options struct {
	EmptyGrace time.Duration
	MaxAge     time.Duration
	MaxMembers int
}

// Hub ist die Registry aller offenen Räume.
type Hub struct {
	opts Options

	mu      sync.Mutex
	byID    map[string]*Room
	byToken map[string]*Room
	byCode  map[string]*Room

	now func() time.Time // in Tests überschreibbar
}

func NewHub(opts Options) *Hub {
	if opts.EmptyGrace <= 0 {
		opts.EmptyGrace = time.Minute
	}
	if opts.MaxAge <= 0 {
		opts.MaxAge = 12 * time.Hour
	}
	if opts.MaxMembers < 2 {
		opts.MaxMembers = 16
	}
	return &Hub{
		opts:    opts,
		byID:    map[string]*Room{},
		byToken: map[string]*Room{},
		byCode:  map[string]*Room{},
		now:     time.Now,
	}
}

// Create legt einen Raum an: interne ID, langer Token für den QR-Code und ein
// 3-Wörter-Code, der unter den offenen Räumen eindeutig ist. lang bestimmt
// die Wortliste und die Sprache der generischen Gerätenamen; eine unbekannte
// oder leere Angabe fällt auf Englisch zurück.
func (h *Hub) Create(ownerID, lang string) (*Room, error) {
	id, err := randomID(12)
	if err != nil {
		return nil, err
	}
	token, err := randomID(32)
	if err != nil {
		return nil, err
	}
	lang = resolveLang(lang)

	h.mu.Lock()
	defer h.mu.Unlock()

	// Der Wörter-Namensraum ist klein, aber es sind immer nur wenige Räume
	// offen — ein paar Versuche reichen also sicher. Kandidaten aus
	// verschiedenen Sprachlisten landen in derselben byCode-Map; eine
	// zufällige Kollision zwischen einem englischen und einem deutschen Code
	// fängt dieser Retry-Loop ohne Zusatzlogik mit ab.
	var code string
	for i := 0; i < 100; i++ {
		c, err := newWordCode(lang)
		if err != nil {
			return nil, err
		}
		if _, taken := h.byCode[c]; !taken {
			code = c
			break
		}
	}
	if code == "" {
		return nil, errors.New("kein freier Beitrittscode verfügbar")
	}

	r := &Room{
		ID:         id,
		Token:      token,
		Code:       code,
		OwnerID:    ownerID,
		Lang:       lang,
		Created:    h.now(),
		members:    map[*Member]struct{}{},
		emptySince: h.now(),
		maxMembers: h.opts.MaxMembers,
		now:        h.now,
	}
	h.byID[r.ID] = r
	h.byToken[r.Token] = r
	h.byCode[r.Code] = r
	return r, nil
}

// ByToken löst den QR-Token auf.
func (h *Hub) ByToken(token string) (*Room, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.byToken[token]; ok {
		return r, nil
	}
	return nil, ErrRoomNotFound
}

// ByCode löst den 3-Wörter-Code auf; die Eingabe wird vorher normalisiert.
func (h *Hub) ByCode(code string) (*Room, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.byCode[NormalizeCode(code)]; ok {
		return r, nil
	}
	return nil, ErrRoomNotFound
}

// ByID liefert einen Raum anhand der internen ID.
func (h *Hub) ByID(id string) (*Room, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.byID[id]; ok {
		return r, nil
	}
	return nil, ErrRoomNotFound
}

// Count ist die Zahl der offenen Räume.
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.byID)
}

// remove nimmt den Raum aus allen Indizes; Token und Code sind damit sofort
// wieder frei.
func (h *Hub) remove(r *Room) {
	delete(h.byID, r.ID)
	delete(h.byToken, r.Token)
	delete(h.byCode, r.Code)
}

// Collect räumt leere und überalterte Räume weg und gibt zurück, wie viele
// es waren.
func (h *Hub) Collect() int {
	now := h.now()

	h.mu.Lock()
	var doomed []*Room
	for _, r := range h.byID {
		r.mu.Lock()
		empty := len(r.members) == 0 && !r.emptySince.IsZero() && now.Sub(r.emptySince) >= h.opts.EmptyGrace
		tooOld := now.Sub(r.Created) >= h.opts.MaxAge
		r.mu.Unlock()
		if empty || tooOld {
			doomed = append(doomed, r)
			h.remove(r)
		}
	}
	h.mu.Unlock()

	for _, r := range doomed {
		r.close()
	}
	return len(doomed)
}

// Run lässt den GC laufen, bis der Kanal schließt.
func (h *Hub) Run(done <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			h.Collect()
		}
	}
}

// NewMemberID erzeugt eine ID für eine neue Verbindung.
func NewMemberID() (string, error) { return randomID(8) }

func randomID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("Zufallswert: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
