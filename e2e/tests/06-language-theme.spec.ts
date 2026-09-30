import { expect, test, type Page } from '@playwright/test'

import { api, cf, login, startReceiver } from './helpers'

const isDark = (page: Page) => page.evaluate(() => document.documentElement.classList.contains('dark'))

test.describe('Sprache', () => {
  test('Umschalten auf Englisch: Oberfläche und Server-Meldungen, bleibt nach Neuladen', async ({ page }) => {
    await page.goto('/login')
    await expect(page.locator('[data-slot=card-title]')).toHaveText('Anmelden')
    await page.getByRole('group', { name: 'Sprache' }).getByRole('button', { name: 'en' }).click()
    await expect(page.locator('[data-slot=card-title]')).toHaveText('Sign in')
    expect(await page.evaluate(() => document.documentElement.lang)).toBe('en')

    // Server-Meldung in der gewählten Sprache
    await page.fill('input[name=password]', 'falsch')
    await page.click('button[type=submit]')
    await expect(page.getByRole('alert')).toContainText('Wrong password')

    await page.fill('input[name=password]', 'test')
    await page.click('button[type=submit]')
    await expect(page).toHaveURL('/')
    await expect(page.getByText('Public IP')).toBeVisible()

    await page.reload()
    await expect(page.getByRole('link', { name: 'History' })).toBeVisible()

    // Gespeicherte Record-Meldung wird je Anfrage übersetzt
    const zone = (await api<{ id: string; name: string }[]>(page, 'GET', '/api/zones')).json[0]
    const rec = (
      await api<{ id: number }>(page, 'POST', '/api/records', {
        zone_id: zone.id, name: `sprache.${zone.name}`, type: 'A', proxied: false, ttl: 1, enabled: true,
      })
    ).json
    await cf.fail(500)
    await api(page, 'POST', `/api/records/${rec.id}/sync`)
    await cf.fail(0)
    await page.goto('/records')
    const row = page.getByRole('row', { name: new RegExp(`sprache\\.${zone.name}`) })
    await expect(row.getByText('Error', { exact: true })).toBeVisible()
    await expect(row.getByText(/Cloudflare API: HTTP 500/)).toBeVisible()

    await page.getByRole('group', { name: 'Language' }).getByRole('button', { name: 'de' }).click()
    await expect(row.getByText(/Cloudflare-API: HTTP 500/)).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Records' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Verlauf' })).toBeVisible()

    await api(page, 'DELETE', `/api/records/${rec.id}`)
  })

  test.describe('englischer Browser', () => {
    test.use({ locale: 'en-US' })
    test('Standard folgt der Browsersprache', async ({ page }) => {
      await page.goto('/login')
      await expect(page.locator('[data-slot=card-title]')).toHaveText('Sign in')
    })
  })

  test('Benachrichtigungen in der eingestellten Sprache', async ({ page }) => {
    const receiver = await startReceiver()
    await login(page)
    const settings = (await api<Record<string, unknown>>(page, 'GET', '/api/settings')).json
    expect((await api(page, 'PUT', '/api/settings', { ...settings, notify_language: 'en' })).status).toBe(200)
    const wh = (
      await api<{ id: number }>(page, 'POST', '/api/webhooks', {
        name: 'Englisch', enabled: true, events: ['tunnel_status'], method: 'POST', url: receiver.url,
        headers: [], body_template: '', content_type: 'application/json',
      })
    ).json

    await cf.tunnel('home', 'down')
    await api(page, 'POST', '/api/tunnels/refresh')
    await expect.poll(() => receiver.received.length).toBeGreaterThan(0)
    expect(JSON.parse(receiver.received.at(-1)!.body)).toMatchObject({ title: 'Tunnel home: disconnected' })

    await cf.tunnel('home', 'healthy')
    await api(page, 'POST', '/api/tunnels/refresh')
    await api(page, 'DELETE', `/api/webhooks/${wh.id}`)
    await api(page, 'PUT', '/api/settings', settings)
    receiver.close()
  })
})

test.describe('Hell/Dunkel', () => {
  test.describe('helles System', () => {
    test.use({ colorScheme: 'light' })
    test('System folgt der Vorgabe, auch live', async ({ page }) => {
      await page.goto('/login')
      expect(await isDark(page)).toBe(false)
      await page.emulateMedia({ colorScheme: 'dark' })
      await expect.poll(() => isDark(page)).toBe(true)
      await page.emulateMedia({ colorScheme: 'light' })
      await expect.poll(() => isDark(page)).toBe(false)
    })
  })

  test.describe('dunkles System', () => {
    test.use({ colorScheme: 'dark' })
    test('manuelle Wahl bleibt nach Neuladen erhalten', async ({ page }) => {
      await page.goto('/login')
      expect(await isDark(page)).toBe(true)
      // System → Hell
      await page.getByRole('button', { name: 'Darstellung: System' }).click()
      await expect.poll(() => isDark(page)).toBe(false)
      await page.reload()
      expect(await isDark(page)).toBe(false)
      await expect(page.getByRole('button', { name: 'Darstellung: Hell' })).toBeVisible()
      // Hell → Dunkel → System
      await page.getByRole('button', { name: 'Darstellung: Hell' }).click()
      await expect.poll(() => isDark(page)).toBe(true)
      await page.getByRole('button', { name: 'Darstellung: Dunkel' }).click()
      await page.emulateMedia({ colorScheme: 'light' })
      await expect.poll(() => isDark(page)).toBe(false)
    })
  })
})
