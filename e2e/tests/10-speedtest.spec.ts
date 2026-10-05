import { expect, test } from '@playwright/test'

import { api, expectNoHorizontalOverflow, login } from './helpers'

// Ohne echte Messung: die würde speed.cloudflare.com belasten und hinge vom
// Netz der Testumgebung ab. Messung und Zeitplan prüfen die Go-Tests.

test.beforeEach(async ({ page }) => {
  await login(page)
})

test('Speedtest: Seite, Dashboard-Kachel, Zeitplan in den Einstellungen', async ({ page }) => {
  await page.getByRole('link', { name: 'Speedtest' }).first().click()
  await expect(page).toHaveURL('/speedtest')
  await expect(page.getByRole('heading', { name: 'Speedtest' })).toBeVisible()
  await expect(page.getByText('Noch keine Messung – starte die erste mit „Jetzt messen“.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Jetzt messen' })).toBeEnabled()
  await expect(page.getByText('Nur manuell')).toBeVisible()

  await page.goto('/')
  await expect(page.getByText('Noch keine Messung')).toBeVisible()

  await page.goto('/einstellungen')
  await page.locator('#speedtest-interval').click()
  await page.getByRole('option', { name: '6 Stunden' }).click()
  await page.getByRole('button', { name: 'Speichern' }).click()
  await expect(page.getByText('Gespeichert')).toBeVisible()

  const s = (await api<{ speedtest_interval_seconds: number }>(page, 'GET', '/api/settings')).json
  expect(s.speedtest_interval_seconds).toBe(21600)
  await page.goto('/speedtest')
  await expect(page.getByText('Automatisch alle 6 Stunden')).toBeVisible()

  // Ausgangszustand wiederherstellen
  expect((await api(page, 'PUT', '/api/settings', { ...s, speedtest_interval_seconds: 0 })).status).toBe(200)

  await page.setViewportSize({ width: 390, height: 844 })
  await expectNoHorizontalOverflow(page)
})
