import { expect, test } from '@playwright/test'

import { api, cf, expectNoHorizontalOverflow, login } from './helpers'

test.beforeEach(async ({ page }) => {
  await login(page)
})

test('Verlauf: Reiter und Filter', async ({ page }) => {
  // Ereignisse erzeugen: Fehler + Tunnel-Wechsel
  const zone = (await api<{ id: string; name: string }[]>(page, 'GET', '/api/zones')).json[0]
  const rec = (await api<{ id: number }>(page, 'POST', '/api/records', {
    zone_id: zone.id, name: `verlauf.${zone.name}`, type: 'A', proxied: false, ttl: 1, enabled: true,
  })).json
  await cf.fail(500)
  await api(page, 'POST', `/api/records/${rec.id}/sync`)
  await cf.fail(0)
  await cf.tunnel('nas', 'down')
  await api(page, 'POST', '/api/tunnels/refresh')
  await cf.tunnel('nas', 'degraded')
  await api(page, 'POST', '/api/tunnels/refresh')

  await page.goto('/verlauf')
  await expect(page.getByText(`verlauf.${zone.name}`).first()).toBeVisible()

  await page.getByRole('combobox', { name: 'Ergebnis' }).click()
  await page.getByRole('option', { name: 'Fehler' }).click()
  const badges = page.locator('ul li [data-slot=badge]:first-child')
  await expect(badges.first()).toHaveText('Fehler')
  expect(new Set(await badges.allTextContents())).toEqual(new Set(['Fehler']))

  await page.getByRole('tab', { name: 'Tunnel-Status' }).click()
  await expect(page.locator('li', { hasText: 'nas' }).filter({ hasText: 'Getrennt' }).first()).toBeVisible()

  await page.getByRole('tab', { name: 'IP-Wechsel' }).click()
  await expect(page.getByText('erstmals erkannt').first()).toBeVisible()

  await page.setViewportSize({ width: 390, height: 844 })
  await expectNoHorizontalOverflow(page)
})

test('Einstellungen: speichern, Mindestanzahl Quellen, Verwerfen', async ({ page }) => {
  await page.goto('/einstellungen')
  const save = page.getByRole('button', { name: 'Speichern' })
  await expect(save).toBeDisabled()

  await page.locator('#ip-interval').click()
  await page.getByRole('option', { name: '2 Minuten' }).click()
  await page.getByRole('button', { name: 'icanhazip nach oben' }).click()
  await page.locator('#src-ipify').click()
  await save.click()
  await expect(page.getByText('Gespeichert')).toBeVisible()

  const s = (await api<{ ip_check_interval_seconds: number; ip_sources: { name: string; enabled: boolean }[] }>(
    page, 'GET', '/api/settings')).json
  expect(s.ip_check_interval_seconds).toBe(120)
  expect(s.ip_sources.map((x) => `${x.name}:${x.enabled}`)).toEqual(['cloudflare:true', 'icanhazip:true', 'ipify:false'])

  await page.locator('#src-icanhazip').click()
  await expect(page.getByText('Mindestens 2 Quellen aktivieren.')).toBeVisible()
  await expect(save).toBeDisabled()
  await page.getByRole('button', { name: 'Verwerfen' }).click()
  await expect(page.getByText('Mindestens 2 Quellen aktivieren.')).toBeHidden()

  // Ausgangszustand wiederherstellen
  s.ip_check_interval_seconds = 300
  s.ip_sources = s.ip_sources.map((x) => ({ ...x, enabled: true }))
  expect((await api(page, 'PUT', '/api/settings', s)).status).toBe(200)

  await expect(page.getByText('dev', { exact: true })).toBeVisible() // Version unter System
  await page.setViewportSize({ width: 390, height: 844 })
  await expectNoHorizontalOverflow(page)
})
