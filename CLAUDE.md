# drop — Projektkontext

## Warum
Ephemeres, selbst gehostetes Tool, um schnell Text, Links, Passwörter und
Dateien zwischen Geräten (Handy ↔ Rechner) zu schieben. Nichts wird
gespeichert. Läuft in der eigenen Homelab-Infrastruktur (kanonenwiese.de)
hinter Nginx Proxy Manager.

## Was
Nutzer meldet sich per OIDC an und erstellt einen **Raum**. Andere treten
per **QR-Code** oder **3 deutschen Wörtern** bei — ohne Login. Im Raum:
eine **Live-Textbox** (einer tippt, Rest liest, Last-Write-Wins) und ein
**Transfer-Feed** für Text-Snippets und Dateien (Paste-Button,
Datei-Button, Drag&Drop; Copy/Download bei empfangenen Items). Installierbar
als PWA.

Die vollständige Spezifikation steht in **SPEC.md** — bei Architektur- oder
Verhaltensfragen dort nachsehen; SPEC.md gewinnt bei Konflikten.

## Wie
- **Ein Binary, ein Port.** Kein WebRTC/TURN. Alles über WebSockets; der
  Server relayt zwischen den Mitgliedern. Server-Vertrauen genügt, kein E2E.
- **Stack:** Go, `embed.FS` (Frontend + Wortliste + Icons + Manifest + SW),
  `coder/websocket`, `coreos/go-oidc` + `golang.org/x/oauth2`.
- **Auth:** OIDC (Pocket ID), Authorization Code + PKCE, signiertes
  Session-Cookie; WS-Upgrade prüft das Cookie. Kein Tinyauth.
- **Räume nur im RAM**, GC wenn leer. Keine DB, kein Volume.
- Frontend in Vanilla-JS. Farben als CSS-Custom-Properties aus dem noch zu
  liefernden Basis-CSS (nicht hart kodieren).

## Konventionen (Homelab)
- Repo unter github.com/doenke; läuft als lokaler Compose-Build
  (`build: https://github.com/doenke/drop.git#main`), Image `drop-drop:latest`.
- Deployment: Bridge-Netz, hinter NPM, `drop.kanonenwiese.de`, Force-SSL,
  WebSockets-Support an.
- Volume-Pfad-Konvention im Homelab: `/data/container/[name]/config` —
  hier aber **kein Volume nötig**.

## Arbeitsweise
- Erst **erkunden und planen** (Plan-Mode), dann implementieren — nicht
  direkt drauflos bauen.
- Git von Anfang an; kleine, nachvollziehbare Commits.
- Secrets (OIDC-Client-Secret etc.) nur über Env/`.env`, niemals ins Repo.

## Verifikation
- `go build ./...` muss durchlaufen.
- `go vet ./...` sauber.
- Manueller Smoke-Test: Login → Raum erstellen → mit zweitem Gerät per
  Code beitreten → Text tippen (erscheint drüben) → Datei per Drag&Drop
  senden (Download drüben) → Paste-Button → Copy-Button.
