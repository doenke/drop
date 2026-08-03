/*
 * Service Worker — nur fuer die App-Shell und das Installier-Angebot.
 *
 * Die Uebertragung selbst laeuft ueber die WebSocket-Verbindung; die faengt
 * ein Service Worker nicht ab und soll er auch nicht. Entsprechend wird hier
 * nichts von /ws, /api oder /auth angefasst.
 */

const VERSION = 'drop-v2';
const SHELL = [
  '/',
  '/static/app.js',
  '/static/style.css',
  '/static/theme.css',
  '/manifest.json',
  '/icons/icon.svg',
  '/icons/icon-192.png',
  '/icons/icon-512.png',
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(VERSION)
      .then((cache) => cache.addAll(SHELL))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== VERSION).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener('fetch', (event) => {
  const request = event.request;
  if (request.method !== 'GET') return;

  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith('/ws')
    || url.pathname.startsWith('/api')
    || url.pathname.startsWith('/auth')) {
    return;
  }

  // Navigationen zuerst aus dem Netz: nur so sieht man nach einem Deploy die
  // neue Shell. Offline bleibt die gecachte Startseite als Rueckfalloption.
  if (request.mode === 'navigate') {
    event.respondWith(
      fetch(request).catch(() => caches.match('/', { ignoreSearch: true })),
    );
    return;
  }

  // Statisches aus dem Cache ausliefern und im Hintergrund auffrischen.
  event.respondWith(
    caches.match(request).then((cached) => {
      const network = fetch(request).then((response) => {
        if (response && response.ok) {
          const copy = response.clone();
          caches.open(VERSION).then((cache) => cache.put(request, copy));
        }
        return response;
      }).catch(() => cached);
      return cached || network;
    }),
  );
});
