import { expect, test } from '@playwright/test'

import { login } from './helpers'

test('PWA: Manifest, Icons, Service Worker', async ({ page, request }) => {
  const res = await request.get('/manifest.webmanifest')
  expect(res.headers()['content-type']).toBe('application/manifest+json')
  const manifest = (await res.json()) as { display: string; icons: { src: string; purpose?: string }[] }
  expect(manifest.display).toBe('standalone')
  expect(manifest.icons.some((i) => i.purpose === 'maskable')).toBe(true)
  for (const icon of manifest.icons) {
    const r = await request.get(icon.src)
    expect(r.status(), icon.src).toBe(200)
    expect(r.headers()['content-type']).toBe('image/png')
  }

  await login(page)
  await expect(page.locator('link[rel=manifest]')).toHaveAttribute('href', '/manifest.webmanifest')

  // Service Worker ist aktiv und an den Build gebunden
  const script = await page.evaluate(async () => {
    const reg = await navigator.serviceWorker.ready
    return reg.active?.scriptURL ?? ''
  })
  expect(script).toMatch(/\/sw\.js\?build=index-.+\.js$/)
})

test('PWA: Oberfläche lädt offline, API wird nie zwischengespeichert', async ({ page, context }) => {
  await login(page)
  await page.evaluate(() => navigator.serviceWorker.ready)
  // Erst nach Übernahme durch den Worker landen Seite und Assets im Cache
  await page.reload()
  await expect(page.getByText('Öffentliche IP')).toBeVisible()

  const cached = await page.evaluate(async () => {
    const urls: string[] = []
    for (const name of await caches.keys()) {
      for (const req of await (await caches.open(name)).keys()) urls.push(new URL(req.url).pathname)
    }
    return urls
  })
  expect(cached).toContain('/')
  expect(cached.some((u) => u.startsWith('/assets/'))).toBe(true)
  expect(cached.some((u) => u.startsWith('/api/'))).toBe(false)

  await context.setOffline(true)
  try {
    await page.reload()
    await expect(page).toHaveTitle('dnsdeck')
    await expect(page.locator('#root')).not.toBeEmpty()
  } finally {
    await context.setOffline(false)
  }
})
