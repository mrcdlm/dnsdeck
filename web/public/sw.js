// Service Worker: macht dnsdeck installierbar und lädt die Oberfläche auch
// offline. Daten (/api) werden nie zwischengespeichert – sie enthalten den
// aktuellen Stand und hängen an der Session.
//
// Registriert wird /sw.js?build=<Hash des Haupt-Bundles> (siehe src/main.tsx):
// Jeder neue Build ergibt einen neuen Worker mit eigenem Cache, alte Caches
// samt veralteter Assets werden beim Aktivieren gelöscht.
const CACHE = 'dnsdeck-' + (new URL(self.location.href).searchParams.get('build') || 'dev')

self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE).then((c) => c.add('/')))
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((names) => Promise.all(names.filter((n) => n !== CACHE).map((n) => caches.delete(n))))
      .then(() => self.clients.claim()),
  )
})

self.addEventListener('fetch', (event) => {
  const req = event.request
  const url = new URL(req.url)
  if (req.method !== 'GET' || url.origin !== self.location.origin) return
  if (url.pathname.startsWith('/api/') || url.pathname === '/healthz') return

  if (req.mode === 'navigate') {
    // Seiten: immer frisch vom Server, offline die zuletzt geladene App-Hülle.
    event.respondWith(
      fetch(req)
        .then((res) => {
          if (res.ok) {
            const copy = res.clone()
            void caches.open(CACHE).then((c) => c.put('/', copy))
          }
          return res
        })
        .catch(() => caches.match('/').then((res) => res || Response.error())),
    )
    return
  }

  if (url.pathname.startsWith('/assets/')) {
    // Assets tragen einen Content-Hash und ändern sich nie.
    event.respondWith(
      caches.match(req).then(
        (hit) =>
          hit ||
          fetch(req).then((res) => {
            if (res.ok) {
              const copy = res.clone()
              void caches.open(CACHE).then((c) => c.put(req, copy))
            }
            return res
          }),
      ),
    )
    return
  }

  // Übrige statische Dateien (Icons, theme-init.js): Netz zuerst, offline aus dem Cache.
  event.respondWith(
    fetch(req)
      .then((res) => {
        if (res.ok) {
          const copy = res.clone()
          void caches.open(CACHE).then((c) => c.put(req, copy))
        }
        return res
      })
      .catch(() => caches.match(req).then((res) => res || Response.error())),
  )
})
