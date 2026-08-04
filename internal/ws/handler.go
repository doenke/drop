package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/doenke/drop/internal/auth"
	"github.com/doenke/drop/internal/httpx"
	"github.com/doenke/drop/internal/room"
)

const (
	writeTimeout = 30 * time.Second
	// Der Nginx Proxy Manager kappt stille Verbindungen nach einer Minute,
	// deshalb hält ein Ping die Leitung offen.
	pingInterval = 25 * time.Second
)

// Limits bündelt die konfigurierbaren Obergrenzen.
type Limits struct {
	MaxFileSize     int64
	MaxChunkSize    int64
	MaxTextItemSize int64
	MaxLiveTextSize int64
}

// Handler bedient /ws.
type Handler struct {
	hub     *room.Hub
	signer  *auth.Signer
	limiter *httpx.Limiter
	limits  Limits
	log     *slog.Logger

	originHost string
	roomURL    func(token string) string
	trustProxy bool
}

func New(hub *room.Hub, signer *auth.Signer, limiter *httpx.Limiter, limits Limits,
	originHost string, roomURL func(string) string, trustProxy bool, log *slog.Logger) *Handler {
	return &Handler{
		hub:        hub,
		signer:     signer,
		limiter:    limiter,
		limits:     limits,
		log:        log,
		originHost: originHost,
		roomURL:    roomURL,
		trustProxy: trustProxy,
	}
}

// conn hält den Zustand einer einzelnen Verbindung.
type conn struct {
	h      *Handler
	ws     *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc

	session   auth.Session
	loggedIn  bool
	clientIP  string
	member    *room.Member
	room      *room.Room
	uploads   map[string]*upload
	writerRun bool
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{h.originHost},
	})
	if err != nil {
		h.log.Debug("WebSocket-Upgrade abgelehnt", "err", err)
		return
	}
	defer c.CloseNow()

	// Etwas Luft über der Chunk-Größe für den ID-Kopf und JSON-Overhead.
	c.SetReadLimit(h.limits.MaxChunkSize + 8<<10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cn := &conn{
		h:        h,
		ws:       c,
		ctx:      ctx,
		cancel:   cancel,
		clientIP: httpx.ClientIP(r, h.trustProxy),
		uploads:  map[string]*upload{},
	}
	if sess, err := h.signer.Read(r); err == nil {
		cn.session, cn.loggedIn = sess, true
	}
	defer cn.leave()

	cn.readLoop()
}

func (c *conn) readLoop() {
	for {
		typ, data, err := c.ws.Read(c.ctx)
		if err != nil {
			return
		}
		if typ == websocket.MessageBinary {
			if err := c.handleBinary(data); err != nil {
				return
			}
			continue
		}
		var msg clientMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			c.fail(errInvalidJSON, "Message is not valid JSON")
			continue
		}
		if err := c.dispatch(msg); err != nil {
			return
		}
	}
}

func (c *conn) dispatch(msg clientMsg) error {
	switch msg.Type {
	case msgCreate:
		return c.handleCreate(msg)
	case msgJoin:
		return c.handleJoin(msg)
	case msgTextSync:
		return c.handleTextSync(msg)
	case msgItemText:
		return c.handleItemText(msg)
	case msgFileMeta:
		return c.handleFileMeta(msg)
	case msgFileEnd:
		return c.handleFileEnd(msg)
	default:
		c.fail(errUnknownType, "Unknown message type")
		return nil
	}
}

func (c *conn) handleCreate(msg clientMsg) error {
	if c.member != nil {
		c.fail(errAlreadyInRoom, "This connection is already in a room")
		return nil
	}
	// Räume anlegen darf nur, wer angemeldet ist; beitreten darf jeder mit
	// Code.
	if !c.loggedIn {
		c.fail(errUnauthorized, "Please sign in to create a room")
		return nil
	}
	r, err := c.h.hub.Create(c.session.Subject, msg.Lang)
	if err != nil {
		c.h.log.Error("Raum anlegen fehlgeschlagen", "err", err)
		c.fail(errCreateFailed, "Could not create room")
		return nil
	}
	return c.enter(r, true)
}

func (c *conn) handleJoin(msg clientMsg) error {
	if c.member != nil {
		c.fail(errAlreadyInRoom, "This connection is already in a room")
		return nil
	}
	// Das Rate-Limit gilt für jeden Beitrittsversuch: der Wörter-Code ist
	// kurz genug, dass Durchprobieren sonst lohnen würde.
	if !c.h.limiter.Allow(c.clientIP) {
		c.fail(errRateLimited, "Too many attempts, please wait a moment")
		return nil
	}

	var (
		r   *room.Room
		err error
	)
	switch {
	case msg.Token != "":
		r, err = c.h.hub.ByToken(msg.Token)
	case strings.TrimSpace(msg.Code) != "":
		r, err = c.h.hub.ByCode(msg.Code)
	default:
		c.fail(errMissingCodeOrToken, "Neither code nor token given")
		return nil
	}
	if err != nil {
		c.fail(errNotFound, "This room is not (or no longer) open")
		return nil
	}
	return c.enter(r, false)
}

// enter nimmt die Verbindung in den Raum auf, startet den Schreiber und
// schickt den Raum-Zustand.
func (c *conn) enter(r *room.Room, created bool) error {
	id, err := room.NewMemberID()
	if err != nil {
		return err
	}
	owner := c.loggedIn && c.session.Subject == r.OwnerID
	m, err := r.Join(id, owner)
	switch {
	case errors.Is(err, room.ErrRoomFull):
		c.fail(errRoomFull, "This room is full")
		return nil
	case errors.Is(err, room.ErrRoomClosed):
		c.fail(errNotFound, "This room is not (or no longer) open")
		return nil
	case err != nil:
		return err
	}

	c.room, c.member = r, m
	c.startWriter()

	text, seq := r.Text()
	peers := make([]peer, 0, r.MemberCount())
	for _, other := range r.Members() {
		if other != m {
			peers = append(peers, asPeer(other))
		}
	}
	c.send(roomMsg{
		Type:    msgRoom,
		ID:      r.ID,
		Token:   r.Token,
		Code:    r.Code,
		URL:     c.h.roomURL(r.Token),
		You:     asPeer(m),
		Peers:   peers,
		Created: created,
		Text:    text,
		TextSeq: seq,
	})
	r.Broadcast(m, frame(peerMsg{Type: msgPeerJoined, Peer: asPeer(m), Members: r.MemberCount()}))
	return nil
}

func (c *conn) handleTextSync(msg clientMsg) error {
	if c.member == nil {
		c.fail(errNotInRoom, "Join a room first")
		return nil
	}
	if msg.Full == nil {
		c.fail(errTextSyncEmpty, "text-sync without content")
		return nil
	}
	if int64(len(*msg.Full)) > c.h.limits.MaxLiveTextSize {
		c.fail(errLiveTextTooLarge, "The live text box content is too long")
		return nil
	}
	seq := c.room.SetText(*msg.Full)
	c.room.Broadcast(c.member, frame(textSyncMsg{
		Type: msgTextSync,
		Full: *msg.Full,
		Seq:  seq,
		From: c.member.ID,
	}))
	return nil
}

func (c *conn) handleItemText(msg clientMsg) error {
	if c.member == nil {
		c.fail(errNotInRoom, "Join a room first")
		return nil
	}
	if msg.Content == "" {
		c.fail(errItemTextEmpty, "item-text without content")
		return nil
	}
	if int64(len(msg.Content)) > c.h.limits.MaxTextItemSize {
		c.fail(errTextItemTooLarge, "The text snippet is too large")
		return nil
	}
	id, err := room.NewMemberID()
	if err != nil {
		return err
	}
	c.room.Broadcast(c.member, frame(itemTextMsg{
		Type:    msgItemText,
		ID:      id,
		Content: msg.Content,
		From:    asPeer(c.member),
	}))
	return nil
}

// leave räumt beim Verbindungsende auf und meldet den Abgang den anderen.
func (c *conn) leave() {
	c.cancel()
	if c.member == nil {
		return
	}
	c.abortUploads()
	r, m := c.room, c.member
	r.Leave(m)
	r.Broadcast(m, frame(peerMsg{Type: msgPeerLeft, Peer: asPeer(m), Members: r.MemberCount()}))
}

// startWriter schickt ab jetzt alles über den Puffer des Members — danach darf
// die Leseschleife nicht mehr selbst schreiben.
func (c *conn) startWriter() {
	c.writerRun = true
	m := c.member
	go func() {
		defer c.cancel()
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-m.Closed():
				reason := "room closed"
				status := websocket.StatusNormalClosure
				if m.TooSlow() {
					reason, status = "connection too slow", websocket.StatusPolicyViolation
				}
				_ = c.ws.Close(status, reason)
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
				err := c.ws.Ping(ctx)
				cancel()
				if err != nil {
					return
				}
			case f := <-m.Out():
				typ := websocket.MessageText
				if f.Binary {
					typ = websocket.MessageBinary
				}
				ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
				err := c.ws.Write(ctx, typ, f.Data)
				cancel()
				if err != nil {
					return
				}
			}
		}
	}()
}

// send schickt eine Nachricht an genau diese Verbindung.
func (c *conn) send(v any) {
	data := encode(v)
	if c.writerRun {
		c.member.Send(room.Frame{Data: data})
		return
	}
	// Vor dem Beitritt gibt es noch keinen Schreiber; hier schreibt nur die
	// Leseschleife, also ist der direkte Weg gefahrlos.
	ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
	defer cancel()
	_ = c.ws.Write(ctx, websocket.MessageText, data)
}

func (c *conn) fail(code, message string) {
	c.send(errorMsg{Type: msgError, Code: code, Message: message})
}

func frame(v any) room.Frame { return room.Frame{Data: encode(v)} }

func asPeer(m *room.Member) peer {
	return peer{ID: m.ID, Name: m.Name, Owner: m.Owner}
}
