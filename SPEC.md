# SPEC — Ephemeres Raum-Transfer-Tool ("drop")

Selbst gehostetes Tool, um schnell Text, Links, Passwörter und Dateien
zwischen Geräten (v. a. Handy ↔ Rechner) zu schieben. Ephemer, nichts
wird gespeichert. Läuft als **ein Binary hinter einem Port** via NPM
über HTTPS.

## Leitprinzipien

- **Kein WebRTC, kein TURN.** Alles läuft über die WebSocket-Verbindung;
  der Server relayt Text und Dateien zwischen den Raum-Mitgliedern.
  Server-Vertrauen genügt (eigene Infrastruktur), **kein E2E**.
- **Ein Port, alles über WebSockets** (Ausnahme: OIDC-Login = HTTP-Redirect).
- **Nichts persistieren.** Räume und Transfers leben nur im RAM;
  Datei-Chunks werden durchgereicht, nicht auf Platte geschrieben.
- **Ein Binary.** Frontend, Wortliste, Icons, Manifest und Service Worker
  via `embed.FS` mit ausgeliefert.

## Authentifizierung

- Login per **OIDC (Pocket ID)**, Authorization Code + PKCE.
- Der Login ist ein normaler HTTP-Redirect-Flow (Browser → Pocket ID →
  Callback), Ergebnis ist ein signiertes **Session-Cookie**.
- Der WebSocket-Upgrade authentifiziert sich über dieses Cookie.
- **Kein Tinyauth** — OIDC ist in-app.

## Räume & Beitritt

- **Erstellen = eingeloggt.** Nur authentifizierte Nutzer können Räume
  anlegen.
- **Beitreten = nur per Code, ohne Login.** Ein zweites Gerät ohne Account
  kann beitreten.
- Beim Erstellen erzeugt der Server:
  - einen internen `roomId`,
  - einen **langen Zufallstoken** für den **QR-Code** (wird gescannt, hohe
    Entropie),
  - einen **3-Wörter-Code** aus einer deutschen Wortliste (menschlicher
    Fallback, tippbar).
- **Entropie entkoppeln:** QR trägt den starken Token; die 3 Wörter sind
  bewusst schwächer und werden abgesichert über: Codes gelten nur für
  *offene* Räume (winziger Namensraum), kurze Gültigkeit, **Rate-Limit**
  auf Join-Versuche.
- Wortliste: kuratierte, kleingeschriebene, tippfreundliche Nomen ohne
  Verwechslungs-/Umlaut-Fallen (deutsche Diceware-Liste als Startpunkt,
  eingedampft). Als Datei im Binary eingebettet.
- QR kodiert die URL, die die Seite **direkt im Raum** öffnet.
- **Mehrere Teilnehmer pro Raum** möglich.

## Raum-Lifecycle (nur RAM)

`erstellt` (eingeloggt) → `offen` (Beitritte per Token/Code) → `leer` →
nach kurzer Grace-Time **GC**. Codes werden beim Schließen sofort wieder
freigegeben. Keine DB, kein Persistenz-Layer.

## WebSocket-Protokoll (Minimal-Set)

Nachrichten tragen ein Typ-Feld.

**Client → Server**
- `create` (nur mit gültiger Session)
- `join { code | token }`
- `text-sync { full }` — Volltext der Live-Box (Last-Write-Wins)
- `item-text { content }` — gepastetes/gedropptes Text-Snippet
- `file-meta { id, name, mime, size }` → `file-chunk { id, seq, bytes }`
  → `file-end { id }`

**Server → die anderen Mitglieder (Fan-out)**
- `peer-joined` / `peer-left`
- gespiegeltes `text-sync`
- `item-text`
- die `file-*`-Frames

**Server-Rollen:** Session bei `create` prüfen; Code/Token → Raum bei
`join` auflösen; Mitglieder-Set pflegen; fan-out; **Backpressure** über
`bufferedAmount`, damit große Dateien den RAM nicht sprengen.

## Frontend / UX

Zwei getrennte Kanäle im Raum:

1. **Live-Textbox (LWW).** Einer tippt, alle sehen es live. Für schnelle
   Links/Notizen. Getippter Text landet **nicht** im Feed.
2. **Transfer-Feed.** Diskrete Items aus Paste / Datei-Button / Drag&Drop.
   Jedes Item bei allen mit passenden Aktionen.

**Aktionen:**
- **Paste-Button:** `navigator.clipboard.read()`. Bild/Blob → als **Datei**
  senden; `text/plain` → als **Text-Item**. Zusätzlich das native
  **Ctrl/Cmd+V-Paste-Event** auf der Seite abfangen (zuverlässiger für
  Screenshots auf dem Desktop). Mobile-Clipboard-Read ist zickig
  (iOS-Safari) — dort ist Textfeld-Paste bzw. Datei-Button der Fallback.
- **Datei senden:** verstecktes `<input type=file multiple>` hinter einem
  Button.
- **Drag&Drop:** Drop-Zone (`dragover`/`drop` → `dataTransfer.files`).
  Gedroppter Text → Text-Item.
- **Copy-Button an empfangenen Items:** `writeText()` für Text,
  `write([ClipboardItem])` für Bilder. Logik: Text → Copy; Bild → Copy +
  Download; sonstige Datei → nur Download (Browser können beliebige Dateien
  nicht in die Zwischenablage legen).

## PWA

- `manifest.json` (Name, Icons, `display: standalone`, `theme_color` /
  `background_color` aus der Palette).
- Kleiner **Service Worker**, der die App-Shell cacht → Installier-Angebot.
  Der SW ist nur für Install/Shell; die Übertragung braucht die Verbindung
  (SW fängt WS nicht ab).
- iOS: „Zum Home-Bildschirm" funktioniert, aber kein Install-Prompt.
- Manifest, SW und Icons kommen aus dem `embed.FS`.

## Design

- **Fancy, in den Kanonenwiese-Farben** (Palette wird als Basis-CSS
  geliefert → als CSS-Custom-Properties übernehmen, nicht hart kodieren).
- Richtung: dunkler Glasmorphismus, farbiger Akzent-Glow auf Buttons und
  der QR-/Code-Karte. Nicht nach Default-Template aussehen lassen.

## Empfohlener Stack

- **Go** als Single-Binary mit `embed.FS`.
- WebSockets: `coder/websocket`.
- OIDC: `coreos/go-oidc` + `golang.org/x/oauth2` (Auth Code + PKCE gegen
  Pocket ID).
- QR: `skip2/go-qrcode` (server) oder client-seitig.
- Frontend: Vanilla-JS (LWW-Box braucht kein Framework).

## Deployment

- Läuft als lokaler Compose-Build aus dem Repo (Muster wie `gardenglow`/
  `autostrom`), Image `drop-drop:latest`.
- Bridge-Netz, hinter NPM, `drop.kanonenwiese.de`, Force-SSL,
  **WebSockets Support** aktiviert.
- Pocket ID: Client anlegen, Redirect-URI = Callback der App.
- Kein Volume (nichts wird gespeichert).

## Nicht-Ziele

- Kein WebRTC/TURN/coturn.
- Keine Persistenz, keine DB.
- Kein E2E (bewusst; Server-Vertrauen genügt).
- Kein gleichzeitiges Editieren (LWW reicht).
