package room

import (
	"errors"
	"testing"
	"time"
)

func testHub(t *testing.T) *Hub {
	t.Helper()
	return NewHub(Options{EmptyGrace: time.Minute, MaxAge: time.Hour, MaxMembers: 3})
}

func TestCreateRegistersAllLookups(t *testing.T) {
	h := testHub(t)
	r, err := h.Create("owner-1", "en")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if r.Token == "" || r.Code == "" || r.ID == "" {
		t.Fatalf("unvollständiger Raum: %+v", r)
	}

	if got, err := h.ByToken(r.Token); err != nil || got != r {
		t.Errorf("ByToken: %v %v", got, err)
	}
	if got, err := h.ByID(r.ID); err != nil || got != r {
		t.Errorf("ByID: %v %v", got, err)
	}
	// Der Code muss auch in der Schreibweise gefunden werden, die ein Nutzer
	// abtippt.
	if got, err := h.ByCode(" " + upper(r.Code) + " "); err != nil || got != r {
		t.Errorf("ByCode mit Tippvariante: %v %v", got, err)
	}
	if _, err := h.ByCode("gibt-es-nicht"); !errors.Is(err, ErrRoomNotFound) {
		t.Errorf("erwartet ErrRoomNotFound, bekommen: %v", err)
	}
}

func TestCreateLangSelectsWordListAndDeviceLabel(t *testing.T) {
	h := testHub(t)

	de, err := h.Create("owner", "de")
	if err != nil {
		t.Fatalf("Create (de): %v", err)
	}
	if de.Lang != "de" {
		t.Fatalf("Lang = %q, erwartet \"de\"", de.Lang)
	}
	m, _ := de.Join(id(t), true)
	if m.Name != "Gerät 1" {
		t.Errorf("Gerätename (de) = %q, erwartet \"Gerät 1\"", m.Name)
	}

	unknown, err := h.Create("owner", "xx")
	if err != nil {
		t.Fatalf("Create (unbekannte Sprache): %v", err)
	}
	if unknown.Lang != "en" {
		t.Fatalf("Lang bei unbekannter Angabe = %q, erwartet \"en\"", unknown.Lang)
	}
	m2, _ := unknown.Join(id(t), true)
	if m2.Name != "Device 1" {
		t.Errorf("Gerätename (Fallback) = %q, erwartet \"Device 1\"", m2.Name)
	}
}

func TestCodesAreUniqueAcrossOpenRooms(t *testing.T) {
	h := testHub(t)
	seen := map[string]bool{}
	for i := 0; i < 300; i++ {
		r, err := h.Create("owner", "en")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if seen[r.Code] {
			t.Fatalf("Code doppelt vergeben: %q", r.Code)
		}
		seen[r.Code] = true
	}
}

func TestJoinLimitAndLeave(t *testing.T) {
	h := testHub(t)
	r, _ := h.Create("owner", "en")

	var members []*Member
	for i := 0; i < 3; i++ {
		m, err := r.Join(id(t), i == 0)
		if err != nil {
			t.Fatalf("Join %d: %v", i, err)
		}
		members = append(members, m)
	}
	if _, err := r.Join(id(t), false); !errors.Is(err, ErrRoomFull) {
		t.Fatalf("erwartet ErrRoomFull, bekommen: %v", err)
	}

	r.Leave(members[0])
	if r.MemberCount() != 2 {
		t.Fatalf("MemberCount = %d, erwartet 2", r.MemberCount())
	}
	select {
	case <-members[0].Closed():
	default:
		t.Fatal("Member wurde beim Leave nicht geschlossen")
	}
	// Doppeltes Leave darf nicht stören.
	r.Leave(members[0])
	if _, err := r.Join(id(t), false); err != nil {
		t.Fatalf("Platz nach Leave nicht frei: %v", err)
	}
}

func TestBroadcastSkipsSender(t *testing.T) {
	h := testHub(t)
	r, _ := h.Create("owner", "en")
	a, _ := r.Join(id(t), true)
	b, _ := r.Join(id(t), false)

	r.Broadcast(a, Frame{Data: []byte("hallo")})

	select {
	case f := <-b.Out():
		if string(f.Data) != "hallo" {
			t.Fatalf("falscher Frame: %q", f.Data)
		}
	default:
		t.Fatal("Empfänger hat nichts bekommen")
	}
	select {
	case f := <-a.Out():
		t.Fatalf("Absender hat sein eigenes Echo bekommen: %q", f.Data)
	default:
	}
}

func TestSendClosesSlowMember(t *testing.T) {
	h := testHub(t)
	r, _ := h.Create("owner", "en")
	m, _ := r.Join(id(t), false)

	for i := 0; i < sendBuffer; i++ {
		if !m.Send(Frame{Data: []byte("x")}) {
			t.Fatalf("Send %d abgelehnt, obwohl Puffer noch Platz hat", i)
		}
	}
	if m.Send(Frame{Data: []byte("zu viel")}) {
		t.Fatal("Send hätte den vollen Puffer melden müssen")
	}
	if !m.TooSlow() {
		t.Fatal("Member ist nicht als zu langsam markiert")
	}
	select {
	case <-m.Closed():
	default:
		t.Fatal("zu langsamer Member wurde nicht geschlossen")
	}
}

func TestTextIsLastWriteWins(t *testing.T) {
	h := testHub(t)
	r, _ := h.Create("owner", "en")

	if _, seq := r.Text(); seq != 0 {
		t.Fatalf("frischer Raum hat Sequenz %d", seq)
	}
	seq1 := r.SetText("erst")
	seq2 := r.SetText("dann")
	if seq2 <= seq1 {
		t.Fatalf("Sequenz wächst nicht: %d → %d", seq1, seq2)
	}
	text, seq := r.Text()
	if text != "dann" || seq != seq2 {
		t.Fatalf("Text = %q/%d, erwartet \"dann\"/%d", text, seq, seq2)
	}
}

func TestCollectRemovesEmptyRoomAfterGrace(t *testing.T) {
	h := testHub(t)
	now := time.Now()
	h.now = func() time.Time { return now }

	r, _ := h.Create("owner", "en")
	m, _ := r.Join(id(t), true)

	// Solange jemand drin ist, wird nicht aufgeräumt (unterhalb von MaxAge).
	now = now.Add(30 * time.Minute)
	if n := h.Collect(); n != 0 {
		t.Fatalf("belegter Raum wurde aufgeräumt (%d)", n)
	}

	r.Leave(m)
	if n := h.Collect(); n != 0 {
		t.Fatalf("Raum wurde vor Ablauf der Grace-Time aufgeräumt (%d)", n)
	}

	// Die Grace-Time läuft ab echter Zeit, deshalb hier direkt setzen.
	r.mu.Lock()
	r.emptySince = now.Add(-2 * time.Minute)
	r.mu.Unlock()

	if n := h.Collect(); n != 1 {
		t.Fatalf("leerer Raum wurde nicht aufgeräumt (%d)", n)
	}
	if h.Count() != 0 {
		t.Fatalf("Hub hält noch %d Räume", h.Count())
	}
	// Token und Code müssen sofort wieder frei sein.
	if _, err := h.ByToken(r.Token); !errors.Is(err, ErrRoomNotFound) {
		t.Errorf("Token noch auflösbar: %v", err)
	}
	if _, err := h.ByCode(r.Code); !errors.Is(err, ErrRoomNotFound) {
		t.Errorf("Code noch auflösbar: %v", err)
	}
}

func TestCollectRemovesOverAgedRoom(t *testing.T) {
	h := testHub(t)
	now := time.Now()
	h.now = func() time.Time { return now }

	r, _ := h.Create("owner", "en")
	m, _ := r.Join(id(t), true)

	now = now.Add(2 * time.Hour) // MaxAge ist eine Stunde
	if n := h.Collect(); n != 1 {
		t.Fatalf("überalterter Raum wurde nicht aufgeräumt (%d)", n)
	}
	select {
	case <-m.Closed():
	default:
		t.Fatal("Mitglied wurde beim Aufräumen nicht getrennt")
	}
	if _, err := r.Join(id(t), false); !errors.Is(err, ErrRoomClosed) {
		t.Fatalf("geschlossener Raum nimmt noch Mitglieder auf: %v", err)
	}
}

func id(t *testing.T) string {
	t.Helper()
	v, err := NewMemberID()
	if err != nil {
		t.Fatalf("NewMemberID: %v", err)
	}
	return v
}

func upper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
	}
	return string(b)
}
