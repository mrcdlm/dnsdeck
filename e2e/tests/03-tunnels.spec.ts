import { expect, test, type Page } from '@playwright/test'

import { api, cf, expectNoHorizontalOverflow, login } from './helpers'

const card = (page: Page, name: string) =>
  page.locator('[data-slot=card]', { has: page.getByText(name, { exact: true }) })

test.beforeEach(async ({ page }) => {
  await login(page)
  await api(page, 'POST', '/api/tunnels/refresh')
})

test.afterEach(async () => {
  await cf.fail(0)
  await cf.tunnel('home', 'healthy')
})

test('Tunnels-Seite: Status, Verbindungen, Colos, Uptime-Balken', async ({ page }) => {
  await page.goto('/tunnels')
  await expect(card(page, 'home').getByText('Verbunden', { exact: true })).toBeVisible()
  await expect(card(page, 'home').getByText('4 Verbindungen')).toBeVisible()
  await expect(card(page, 'home').getByText('FRA06')).toBeVisible()
  await expect(card(page, 'nas').getByText('Eingeschränkt')).toBeVisible()
  await expect(card(page, 'nas').getByText('1 Verbindung', { exact: true })).toBeVisible()
  await expect(card(page, 'backup').getByText('Getrennt', { exact: true })).toBeVisible()

  await expect(card(page, 'home').locator('[role=img] > div')).toHaveCount(48)
  await page.getByRole('button', { name: '7 Tage' }).click()
  await expect(card(page, 'home').locator('[role=img] > div')).toHaveCount(56)
})

test('Live-Update: Statuswechsel erscheint ohne Neuladen in anderem Tab', async ({ page, context }) => {
  await page.goto('/tunnels')
  await expect(card(page, 'home').getByText('Verbunden', { exact: true })).toBeVisible()

  const other = await context.newPage()
  await other.goto('/tunnels')
  await cf.tunnel('home', 'down')
  await other.getByRole('button', { name: 'Jetzt abfragen' }).click()

  // weit unter dem 30-s-Nachladen → kommt über SSE
  await expect(card(page, 'home').getByText('Getrennt', { exact: true })).toBeVisible({ timeout: 5_000 })
  await expect(card(page, 'home').locator('[role=img] > div.bg-destructive').first()).toBeVisible()
})

test('Cloudflare-Fehler: Banner, letzter Stand bleibt sichtbar', async ({ page }) => {
  await page.goto('/tunnels')
  await cf.fail(500)
  await page.getByRole('button', { name: 'Jetzt abfragen' }).click()
  await expect(page.getByText('Letzte Abfrage fehlgeschlagen')).toBeVisible()
  await expect(card(page, 'nas')).toBeVisible()
  await cf.fail(0)
  await page.getByRole('button', { name: 'Jetzt abfragen' }).click()
  await expect(page.getByText('Letzte Abfrage fehlgeschlagen')).toBeHidden()
})

test('Dashboard-Kachel und mobile Ansicht', async ({ page }) => {
  await expect(page.getByText('3 überwacht')).toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/tunnels')
  await expect(card(page, 'home')).toBeVisible()
  await expectNoHorizontalOverflow(page)
})
