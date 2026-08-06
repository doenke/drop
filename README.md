<img src="docs/logo.png" alt="" width="96" height="96">

**Language:** English · [Deutsch](README.de.md)

# drop

drop is your quick drop zone between devices: a place to push text, links,
passwords, and files from your computer to your phone and back, without a
detour through messengers, mail, or cloud storage. No account for the
other side, no history, nothing is stored — as soon as the last person
leaves, the room is gone.

In short: no more emailing yourself, no more screenshots scattered across
three messengers — just a link or three words, and the file is on the
other side.

![drop landing page: create a room or join with three words](docs/screenshot-start.png)

## What drop does for you

- **Open rooms in seconds** – sign in, "Create room", done.
- **Join without a login** – a QR code or three words are enough for the
  second device.
- **Type together, live** – a shared text box where everyone sees what
  comes in instantly. Ideal for a link or a one-time password.
- **Pass files through** – file button, drag & drop, or just Ctrl/Cmd+V
  for a screenshot.
- **Reuse what you receive right away** – copy text, copy or download
  images, download everything else.
- **Install it as an app** – drop is a PWA and can be added to your home
  screen.
- **In English or German** – the interface automatically detects your
  browser's language.

## Who is this for?

drop fits you if you ...

- constantly shuttle links or passwords between your computer and phone,
- don't want to email yourself screenshots,
- need to hand someone a file for a moment without them needing an
  account,
- self-host and want to know where your data lives — here: nowhere,
  because nothing is stored.

## Quick start with Docker

```bash
cp .env.example .env
```

Fill in at least `DROP_PUBLIC_URL`, `DROP_OIDC_ISSUER`,
`DROP_OIDC_CLIENT_ID`, and a `DROP_SESSION_KEY` (see "Login via OIDC"
below).

```yaml
services:
  drop:
    build: https://github.com/doenke/drop.git#main
    image: drop-drop:latest
    container_name: drop
    restart: unless-stopped
    env_file: .env
    ports:
      - "8080:8080"
```
A volume is deliberately not included — drop persists nothing.

Then open `http://localhost:8080`. Creating a room requires signing in
first — joining via code always works, without an account.

## A first walkthrough

### 1. Create a room

Sign in, click "Create room" — done. drop hands out a long random token
for the QR code and a short 3-word code as a typeable fallback.

### 2. Bring in a second device

Scan the QR code or type the three words. No account needed, no extra
step — the link leads straight into the room.

![Open room: QR code, live text box, and transfer feed with text and image](docs/screenshot-raum.png)

### 3. Type live or send

The live text box is meant for quick links and passwords — one person
types, everyone sees it instantly, and it never lands in a history. For
files and larger text snippets, there's the transfer feed: file button,
drag & drop, or paste with Ctrl/Cmd+V.

### 4. Receive and reuse

Every item in the feed has matching actions: copy text, copy or download
images, download everything else. As soon as the last person leaves the
room, everything is gone.

## Login via OIDC

drop signs in room creators via OIDC (Authorization Code + PKCE), for
example against [Pocket ID](https://github.com/pocket-id/pocket-id). For
that you need:

- `DROP_OIDC_ISSUER`
- `DROP_OIDC_CLIENT_ID`

`DROP_OIDC_CLIENT_SECRET` stays empty for a public client. Register
exactly `${DROP_PUBLIC_URL}/auth/callback` as the redirect URI with your
provider.

Joining via QR code or 3-word code needs **no** account — that's meant
for the second device that's only along for a short while.

## Small config, big payoff

For normal operation you only need a few values:

| Setting | What for? |
| --- | --- |
| `DROP_PUBLIC_URL` | Required. The public address that the redirect URI, room links, and the allowed WebSocket origin are built from. |
| `DROP_OIDC_ISSUER` / `DROP_OIDC_CLIENT_ID` | Required. The identity provider for room creators' login. |
| `DROP_SESSION_KEY` | Key for the signed session cookie. If missing, drop generates one at startup — then every restart logs everyone out. |
| `DROP_TRUSTED_PROXY` | `true` if a reverse proxy sits in front and `X-Forwarded-For` carries the real client IP. |
| `DROP_TITLE` | Instance name in the tab, header, installed PWA, and footer. Default: `drop`. |
| `DROP_HEADER_LOGO_URL` | Your own logo in the far left of the header, before the drop icon. Default: no logo. |

All values, including transfer limits, room lifetime, and rate limits,
are documented in detail in [.env.example](.env.example).

---

## Ops corner

The fully commented compose template lives at
[compose.example.yml](compose.example.yml).

### Reverse proxy (e.g. Nginx Proxy Manager)

Point a proxy host at the container, then:

- Enable **WebSockets support** — without it, no connection comes
  through.
- Enable **Force SSL** so the session cookie comes back with `Secure`.
- Set `DROP_TRUSTED_PROXY=true` so the rate limit uses the real client IP
  from `X-Forwarded-For` instead of the proxy's address.

The server pings every 25 seconds so a proxy doesn't cut silent
connections.

### Important operational notes

- `DROP_SESSION_KEY` should be set and at least 16 characters long,
  otherwise drop generates a random key at startup and every restart
  loses all sessions.
- Rooms only apply to a single container — drop keeps its state
  exclusively in process memory and isn't built for multiple replicas
  behind the same load balancer.
- The 3-word code deliberately has little entropy;
  `DROP_JOIN_RATE_PER_MINUTE` and `DROP_JOIN_RATE_BURST` limit
  brute-force attempts per IP.
- `DROP_AVATAR_HOSTS` is the SSRF protection for the avatar proxy — the
  shorter the list, the better.


## License

[MIT](LICENSE)
