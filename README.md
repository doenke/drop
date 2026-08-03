# drop

Ephemeres, selbst gehostetes Tool, um Text, Links, Passwörter und Dateien
schnell zwischen Geräten zu schieben — vor allem Handy ↔ Rechner. Ein Binary,
ein Port, nichts wird gespeichert.

Ein angemeldeter Nutzer erstellt einen **Raum**. Andere treten per **QR-Code**
oder **drei deutschen Wörtern** bei, ohne Login. Im Raum gibt es eine
**Live-Textbox** (einer tippt, alle sehen es sofort) und einen
**Transfer-Feed** für Text-Snippets und Dateien. Installierbar als PWA.

Die vollständige Spezifikation steht in [SPEC.md](SPEC.md).

## Wie es funktioniert

- Alles läuft über **eine WebSocket-Verbindung**; der Server relayt zwischen
  den Mitgliedern. Kein WebRTC, kein TURN, kein E2E — Server-Vertrauen genügt,
  weil es die eigene Infrastruktur ist.
- **Räume leben nur im RAM.** Keine Datenbank, kein Volume. Ein leerer Raum
  wird nach kurzer Grace-Time weggeräumt; Token und Wörter-Code sind dann
  sofort wieder frei.
- **Dateien werden durchgereicht, nicht abgelegt.** Chunks gehen als
  Binärframes rein und unverändert wieder raus.
- Frontend, Wortliste, Icons, Manifest und Service Worker stecken per
  `embed.FS` im Binary.

## Betrieb

```sh
cp .env.example .env      # Werte eintragen, mindestens DROP_PUBLIC_URL,
                          # DROP_OIDC_* und DROP_SESSION_KEY
cp compose.example.yml compose.yml
docker compose up -d --build
```

Alle Einstellungen sind in [.env.example](.env.example) dokumentiert.

### Pocket ID

Einen Client anlegen und als Redirect-URI genau
`${DROP_PUBLIC_URL}/auth/callback` eintragen. Der Login läuft als
Authorization Code mit PKCE; bei einem öffentlichen Client bleibt
`DROP_OIDC_CLIENT_SECRET` leer.

### Nginx Proxy Manager

Proxy Host auf den Container, dazu:

- **Websockets Support** aktivieren — ohne das kommt keine Verbindung zustande.
- **Force SSL** aktivieren, damit das Session-Cookie mit `Secure` zurückkommt.
- `DROP_TRUSTED_PROXY=true` setzen, damit das Rate-Limit die echte Client-IP
  aus `X-Forwarded-For` benutzt statt der Proxy-Adresse.

Der Server pingt alle 25 Sekunden, damit der Proxy stille Verbindungen nicht
kappt.

## Endpunkte

| Pfad | Zweck |
| --- | --- |
| `/` | App-Shell |
| `/r/{token}` | dieselbe Shell, tritt direkt dem Raum bei (Ziel des QR-Codes) |
| `/ws` | WebSocket: Räume anlegen, beitreten, alle Inhalte |
| `/auth/login`, `/auth/callback`, `/auth/logout` | OIDC |
| `/api/me` | Anmeldestatus fürs Frontend |
| `/api/qr?token=…` | QR-Code des Raum-Links als PNG |
| `/healthz` | Liveness |

## Entwicklung

```sh
go build ./...
go vet ./...
go test ./...            # Unit-Tests plus WebSocket-Integrationstest
```

Zum lokalen Start werden `DROP_PUBLIC_URL`, `DROP_OIDC_ISSUER` und
`DROP_OIDC_CLIENT_ID` gebraucht; der Issuer muss beim Start erreichbar sein,
weil drop die OIDC-Discovery sofort durchführt.

Die Farben stehen ausschließlich in [`web/theme.css`](web/theme.css) als
Custom Properties. `web/style.css` greift nur über `var()` darauf zu — ein
neues Basis-CSS zieht also an genau einer Stelle ein.

## Lizenz

[MIT](LICENSE)
