import { expect, test, type Page } from '@playwright/test'

import { api, login } from './helpers'

// Erzeugt die Bilder in docs/screenshots (EN) und docs/screenshots/de.
// Nur auf Anfrage: `npm run screenshots` (läuft nicht in CI).
//
// Echte Werte der Testumgebung (öffentliche IP, Anbieter) werden durch
// Dokumentationsadressen ersetzt, damit nie echte Daten im Repo landen.

test.skip(!process.env.SCREENSHOTS, 'nur mit SCREENSHOTS=1 (npm run screenshots)')
test.use({ colorScheme: 'dark', viewport: { width: 1280, height: 800 } })

const IPV4 = '203.0.113.42'
const IPV6 = '2001:db8:42::1'
const ISP = {
  asn: 64500,
  name: 'Example Broadband',
  prefix: '203.0.113.0/24',
  country: 'DE',
  registry: 'ripencc',
  hostname: 'host-203-0-113-42.example.net',
}

const texts = {
  en: { expires: 'Certificate expires on {date} (9 days left)', timeout: 'Timed out – no response' },
  de: { expires: 'Zertifikat läuft am {date} ab (noch 9 Tage)', timeout: 'Zeitüberschreitung – keine Antwort' },
}

/** Status je Abschnitt, gezählt vom neuesten (0) rückwärts; since = Stunden im aktuellen Status. */
const tunnelHistory = {
  home: { since: 30, at: (i: number, r: string) => (i === (r === '24h' ? 31 : 9) ? 'down' : 'healthy') },
  nas: { since: 3, at: (i: number, r: string) => (i < (r === '24h' ? 6 : 1) ? 'degraded' : 'healthy') },
  backup: { since: 2, at: (i: number, r: string) => (i < (r === '24h' ? 4 : 1) ? 'down' : 'healthy') },
}

const days = (n: number) => new Date(Date.now() + n * 86_400_000).toISOString()

/** Legt Beispieldaten an (einmal je Serverstart). */
async function seed(page: Page) {
  const names = ((await api<{ name: string }[]>(page, 'GET', '/api/records')).json ?? []).map((r) => r.name)
  for (const [zone, name] of [
    ['example.com', 'cloud.example.com'],
    ['example.com', 'vpn.example.com'],
    ['example.com', 'home.example.com'],
    ['example.org', 'nas.example.org'],
  ]) {
    if (names.includes(name)) continue
    const zones = (await api<{ id: string; name: string }[]>(page, 'GET', '/api/zones')).json
    const zone_id = zones.find((z) => z.name === zone)!.id
    await api(page, 'POST', '/api/records', { zone_id, name, type: 'A', proxied: name.startsWith('cloud'), ttl: 1, enabled: true })
  }
  const hooks = ((await api<{ name: string }[]>(page, 'GET', '/api/webhooks')).json ?? []).map((w) => w.name)
  const base = { enabled: true, method: 'POST', headers: [], content_type: 'application/json' }
  if (!hooks.includes('ntfy'))
    await api(page, 'POST', '/api/webhooks', {
      ...base,
      name: 'ntfy',
      url: 'https://ntfy.sh',
      events: ['ip_change', 'update_failed', 'site_down', 'cert_expiring'],
      body_template:
        '{\n  "topic": {{json (env "WEBHOOK_NTFY_TOPIC")}},\n  "title": {{json .Title}},\n  "message": {{json .Message}},\n  "priority": {{.Priority}}\n}',
    })
  if (!hooks.includes('Telegram'))
    await api(page, 'POST', '/api/webhooks', {
      ...base,
      name: 'Telegram',
      url: 'https://api.telegram.org/bot${WEBHOOK_TELEGRAM_TOKEN}/sendMessage',
      events: ['tunnel_status', 'site_down', 'site_recovered'],
      body_template:
        '{\n  "chat_id": {{json (env "WEBHOOK_TELEGRAM_CHAT_ID")}},\n  "text": {{json (printf "%s\\n%s" .Title .Message)}}\n}',
    })
}

/** Ersetzt echte Werte in allen API-Antworten (außer dem Live-Stream). */
async function anonymize(page: Page, lang: 'en' | 'de') {
  const real = (await api<{ ipv4: { ip?: string }; ipv6: { ip?: string } }>(page, 'GET', '/api/ip')).json
  const records = (await api<{ id: number; name: string }[]>(page, 'GET', '/api/records')).json
  const rec = (name: string) => records.find((r) => r.name === name)?.id
  const checked = new Date(Date.now() - 60_000).toISOString()
  const expiry = days(9)
  const probes = [
    { id: 1, record_id: rec('cloud.example.com'), record_name: 'cloud.example.com', url: 'https://cloud.example.com/',
      enabled: true, status: 'up', http_status: 200, latency_ms: 84, fail_count: 0,
      tls_not_after: days(71), tls_issuer: "Let's Encrypt (E6)", tls_valid: true, last_checked_at: checked },
    { id: 2, record_id: rec('vpn.example.com'), record_name: 'vpn.example.com', url: 'https://vpn.example.com/',
      enabled: true, status: 'expiring', http_status: 302, latency_ms: 41, fail_count: 0,
      message: texts[lang].expires.replace('{date}', expiry.slice(0, 10)),
      tls_not_after: expiry, tls_issuer: "Let's Encrypt (R11)", tls_valid: true, last_checked_at: checked },
    { id: 3, url: 'http://192.168.1.20:8123/', enabled: true, status: 'up', http_status: 200, latency_ms: 12,
      fail_count: 0, last_checked_at: checked },
    { id: 4, record_id: rec('nas.example.org'), record_name: 'nas.example.org', url: 'https://nas.example.org/',
      enabled: true, status: 'down', fail_count: 3, message: texts[lang].timeout,
      tls_not_after: days(40), tls_issuer: 'ZeroSSL ECC Domain Secure Site CA', tls_valid: true, last_checked_at: checked },
  ]

  await page.route(/\/api\/(?!events)/, async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/probes' && route.request().method() === 'GET') return route.fulfill({ json: probes })
    const res = await route.fetch()
    let body = await res.text()
    if (path === '/api/tunnels') {
      // Frischer Testserver hat keinen Verlauf – realistische Uptime-Balken einsetzen.
      const o = JSON.parse(body)
      for (const tn of o.tunnels) {
        const pattern = tunnelHistory[tn.name as keyof typeof tunnelHistory]
        if (!pattern) continue
        for (const [range, u] of Object.entries(tn.uptime as Record<string, { buckets: string[] }>)) {
          const n = u.buckets.length
          const buckets = Array.from({ length: n }, (_, i) => pattern.at(n - 1 - i, range))
          const up = buckets.filter((b) => b === 'healthy' || b === 'degraded').length
          Object.assign(u, { buckets, observed: 1, percent: Math.round((up / n) * 1000) / 10 })
        }
        tn.status_since = new Date(Date.now() - pattern.since * 3600_000).toISOString()
        if (tn.conns_active_at) tn.conns_active_at = tn.status_since
      }
      body = JSON.stringify(o)
    }
    if (path === '/api/ip') {
      const s = JSON.parse(body)
      s.ipv4.isp = ISP
      s.ipv6 = { ...s.ipv4, ip: IPV6, isp: { ...ISP, prefix: '2001:db8::/32' }, blocklist: undefined, message: undefined }
      body = JSON.stringify(s)
    }
    for (const [from, to] of [[real.ipv4.ip, IPV4], [real.ipv6.ip, IPV6]]) {
      if (from) body = body.replaceAll(from, to!)
    }
    await route.fulfill({ response: res, body })
  })
}

for (const lang of ['en', 'de'] as const) {
  test.describe(lang, () => {
    test.use({ locale: lang === 'de' ? 'de-DE' : 'en-US' })

    test(`Screenshots ${lang}`, async ({ page }) => {
      const dir = lang === 'de' ? '../docs/screenshots/de' : '../docs/screenshots'
      await login(page)
      await seed(page)
      await anonymize(page, lang)
      const shot = async (name: string) => {
        await page.waitForTimeout(500) // Animationen abwarten
        await page.screenshot({ path: `${dir}/${name}.png` })
      }

      await page.goto('/')
      await expect(page.getByText(IPV4).first()).toBeVisible()
      await expect(page.getByText(ISP.name).first()).toBeVisible()
      await shot('dashboard')

      await page.goto('/records')
      await expect(page.getByText('cloud.example.com').first()).toBeVisible()
      // Verbreitungsprüfung abwarten (der verzögerte Mock-Resolver braucht ~15 s)
      await expect(page.locator('[data-slot=badge] .animate-spin')).toHaveCount(0, { timeout: 60_000 })
      await shot('records')

      await page.goto('/tunnels')
      await expect(page.getByText('backup').first()).toBeVisible()
      await shot('tunnels')

      await page.goto('/erreichbarkeit')
      await expect(page.getByText('https://cloud.example.com/').first()).toBeVisible()
      await shot('checks')

      await page.goto('/einstellungen')
      const edit = lang === 'de' ? 'Telegram bearbeiten' : 'Edit Telegram'
      await page.getByRole('button', { name: edit }).click()
      await expect(page.getByRole('dialog')).toBeVisible()
      await page.locator('#wh-name').evaluate((el) => (el as HTMLInputElement).blur())
      await shot('webhooks')
    })
  })
}
