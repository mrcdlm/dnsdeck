import { expect, test } from '@playwright/test'

import { api, expectNoHorizontalOverflow, login } from './helpers'

test('falsches Passwort zeigt Meldung und leert das Feld', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await page.fill('input[name=password]', 'falsch')
  await page.click('button[type=submit]')
  await expect(page.getByRole('alert')).toContainText('Passwort falsch')
  await expect(page.locator('input[name=password]')).toHaveValue('')
})

test('Login, Reload, Abmelden, geschützte Seiten', async ({ page }) => {
  await login(page)
  await expect(page.getByText('Öffentliche IP')).toBeVisible()

  await page.reload()
  await expect(page).toHaveURL('/')
  await expect(page.getByText('Öffentliche IP')).toBeVisible()

  // unbekannte Route → Dashboard
  await page.goto('/gibtsnicht')
  await expect(page).toHaveURL('/')

  await page.getByRole('button', { name: 'Abmelden' }).first().click()
  await expect(page).toHaveURL(/\/login$/)
  await page.goto('/records')
  await expect(page).toHaveURL(/\/login$/)
  expect((await api(page, 'GET', '/api/ip')).status).toBe(401)
})

test('Dashboard: IP-Karte, Jetzt aktualisieren, Kopieren, Live', async ({ page }) => {
  await login(page)
  await expect(page.getByText('Live', { exact: true })).toBeVisible()

  const before = (await api<{ last_checked?: string; ipv4: { ip?: string } }>(page, 'GET', '/api/ip')).json
  await page.waitForTimeout(1100) // Zeitstempel muss sich messbar unterscheiden
  const done = page.waitForResponse((r) => r.url().endsWith('/api/ip/refresh'))
  await page.getByRole('button', { name: 'Jetzt aktualisieren' }).click()
  expect((await done).status()).toBe(200)
  await expect(page.getByText('Zuletzt geprüft gerade eben')).toBeVisible()
  const after = (await api<{ last_checked?: string; ipv4: { ip?: string } }>(page, 'GET', '/api/ip')).json
  expect(after.last_checked).not.toBe(before.last_checked)

  if (after.ipv4.ip) {
    await page.getByRole('button', { name: 'IPv4-Adresse kopieren' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(after.ipv4.ip)
  }
})

test('Abmelden in einem Tab meldet andere Tabs über den Live-Stream ab', async ({ page, context }) => {
  test.slow() // Session-Prüfung im Heartbeat (25 s)
  await login(page)
  const other = await context.newPage()
  await other.goto('/')
  await other.getByRole('button', { name: 'Abmelden' }).first().click()
  await expect(page).toHaveURL(/\/login$/, { timeout: 45_000 })
})

test('Dashboard mobil ohne Überlauf', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await login(page)
  await expect(page.getByText('Öffentliche IP')).toBeVisible()
  await expectNoHorizontalOverflow(page)
})

test('Dashboard: Sperrlisten-Prüfung der IPv4', async ({ page }) => {
  await login(page)
  // Echte Abfrage – das Ergebnis hängt von der IP der Testumgebung ab.
  const summary = page.locator('summary').filter({ hasText: /Sperrliste|Sperrlisten nicht prüfbar/ })
  await expect(summary).toBeVisible({ timeout: 20_000 })
  await summary.click()
  await expect(page.getByText('Spamhaus ZEN:')).toBeVisible()
  await expect(page.getByText(/^Geprüft /)).toBeVisible()

  const ip = (await api<{ ipv4: { blocklist?: { lists: unknown[] } } }>(page, 'GET', '/api/ip')).json
  expect(ip.ipv4.blocklist?.lists).toHaveLength(4)
})
