import { expect, test, type Page } from '@playwright/test'

import { api, cf, expectNoHorizontalOverflow, login } from './helpers'

// Zeile anhand Name und Typ-Badge (Zeilentext hat keine Leerzeichen, daher Badge prüfen)
const row = (page: Page, name: string, type: string) =>
  page
    .locator('tr')
    .filter({ has: page.getByText(name, { exact: true }) })
    .filter({ has: page.locator('[data-slot=badge]', { hasText: new RegExp(`^${type}$`) }) })

async function addRecord(page: Page, sub: string, zone: string, type?: string) {
  await page.getByRole('button', { name: 'Hinzufügen' }).first().click()
  const dlg = page.getByRole('dialog')
  await dlg.getByRole('combobox', { name: 'Zone' }).click()
  await page.getByRole('option', { name: zone, exact: true }).click()
  await dlg.getByLabel('Name').fill(sub)
  if (type) {
    await dlg.getByRole('combobox', { name: 'Typ' }).click()
    await page.getByRole('option', { name: type }).click()
  }
  await dlg.getByRole('button', { name: 'Hinzufügen' }).click()
  await expect(dlg).toBeHidden()
}

test.beforeEach(async ({ page }) => {
  await login(page)
  await page.goto('/records')
})

test('Record anlegen, Duplikat, bearbeiten, Abweichung korrigieren, entfernen', async ({ page }) => {
  const ip = (await api<{ ipv4: { ip: string } }>(page, 'GET', '/api/ip')).json.ipv4.ip
  test.skip(!ip, 'keine öffentliche IPv4 ermittelbar')

  await addRecord(page, 'home', 'example.com')
  await expect(row(page, 'home.example.com', 'A').getByText('Aktuell')).toBeVisible()
  let rec = (await cf.records()).find((r) => r.name === 'home.example.com')!
  expect(rec).toMatchObject({ content: ip, comment: 'managed by dnsdeck', proxied: false })
  await expect(row(page, 'home.example.com', 'A').getByText(ip)).toBeVisible()

  // Duplikat
  await page.getByRole('button', { name: 'Hinzufügen' }).first().click()
  let dlg = page.getByRole('dialog')
  await dlg.getByRole('combobox', { name: 'Zone' }).click()
  await page.getByRole('option', { name: 'example.com', exact: true }).click()
  await dlg.getByLabel('Name').fill('home')
  await dlg.getByRole('button', { name: 'Hinzufügen' }).click()
  await expect(dlg.getByRole('alert')).toContainText('bereits verwaltet')
  await dlg.getByRole('button', { name: 'Abbrechen' }).click()

  // Proxy in dnsdeck einschalten → zu Cloudflare übertragen
  await row(page, 'home.example.com', 'A').getByRole('button', { name: /bearbeiten/ }).click()
  dlg = page.getByRole('dialog')
  await expect(dlg.getByLabel('Name')).toHaveValue('home')
  await dlg.getByRole('switch', { name: 'Über Cloudflare proxien' }).click()
  await dlg.getByRole('button', { name: 'Speichern' }).click()
  await expect(dlg).toBeHidden()
  await expect.poll(async () => (await cf.records()).find((r) => r.name === 'home.example.com')?.proxied).toBe(true)
  await expect(page.getByText('Proxy: aus → an').first()).toBeVisible()

  // Im Cloudflare-Dashboard: IP falsch → dnsdeck korrigiert nur die IP
  rec = (await cf.records()).find((r) => r.name === 'home.example.com')!
  await cf.patch(rec.zone_id, rec.id, { content: '198.51.100.7' })
  await page.getByRole('button', { name: 'Jetzt aktualisieren' }).click()
  await expect.poll(async () => (await cf.records()).find((r) => r.name === 'home.example.com')?.content).toBe(ip)
  await expect(page.getByText('198.51.100.7').first()).toBeVisible()

  // Entfernen: nur aus dnsdeck
  const count = (await cf.records()).length
  await row(page, 'home.example.com', 'A').getByRole('button', { name: /entfernen/ }).click()
  await expect(page.getByRole('alertdialog')).toContainText('Der Eintrag bei Cloudflare bleibt')
  await page.getByRole('alertdialog').getByRole('button', { name: 'Entfernen' }).click()
  await expect(row(page, 'home.example.com', 'A')).toHaveCount(0)
  expect((await cf.records()).length).toBe(count)
})

test('A + AAAA ohne IPv6: AAAA übersprungen', async ({ page }) => {
  const ipv6 = (await api<{ ipv6: { ip?: string } }>(page, 'GET', '/api/ip')).json.ipv6.ip
  test.skip(!!ipv6, 'Testumgebung hat IPv6')
  await addRecord(page, 'dual', 'example.org', 'A + AAAA')
  await expect(row(page, 'dual.example.org', 'AAAA').getByText('Übersprungen')).toBeVisible()
  await expect(row(page, 'dual.example.org', 'AAAA').getByText('keine öffentliche IPv6-Adresse bekannt')).toBeVisible()
})

test('Cloudflare-Ausfall: Fehler, danach „Behoben“', async ({ page }) => {
  await addRecord(page, 'ausfall', 'example.com')
  const r = row(page, 'ausfall.example.com', 'A')
  await expect(r.getByText('Aktuell')).toBeVisible()
  await cf.fail(500)
  try {
    await r.getByRole('button', { name: /abgleichen/ }).click()
    await expect(r.getByText('Fehler', { exact: true })).toBeVisible()
    await expect(r.getByText('simulierter Fehler')).toBeVisible()
  } finally {
    await cf.fail(0)
  }
  await r.getByRole('button', { name: /abgleichen/ }).click()
  await expect(r.getByText('Aktuell')).toBeVisible()
  await expect(page.getByText(/Abgleich wieder erfolgreich/).first()).toBeVisible()
})

test('Bestehenden Eintrag übernehmen: Proxy von Cloudflare bleibt', async ({ page }) => {
  const zone = (await api<{ id: string; name: string }[]>(page, 'GET', '/api/zones')).json.find((z) => z.name === 'example.com')!
  const ip = (await api<{ ipv4: { ip: string } }>(page, 'GET', '/api/ip')).json.ipv4.ip
  await cf.create(zone.id, { name: 'app.example.com', type: 'A', content: ip, ttl: 1, proxied: true })

  await addRecord(page, 'app', 'example.com') // Dialog-Voreinstellung: Proxy aus
  await expect(row(page, 'app.example.com', 'A').getByLabel('Proxied')).toBeVisible()
  expect((await cf.records()).find((r) => r.name === 'app.example.com')?.proxied).toBe(true)
  await expect(page.getByText('bestehenden Eintrag übernommen (Proxy an, TTL Auto)')).toBeVisible()

  // Proxy im Cloudflare-Dashboard aus → dnsdeck übernimmt, dreht nicht zurück
  const rec = (await cf.records()).find((r) => r.name === 'app.example.com')!
  await cf.patch(zone.id, rec.id, { proxied: false })
  await page.getByRole('button', { name: 'Jetzt aktualisieren' }).click()
  await expect(row(page, 'app.example.com', 'A').getByLabel('Nur DNS')).toBeVisible()
  expect((await cf.records()).find((r) => r.name === 'app.example.com')?.proxied).toBe(false)
})

test('Records mobil als Karten mit Aktionen', async ({ page }) => {
  await addRecord(page, 'mobil', 'example.org')
  await page.setViewportSize({ width: 390, height: 844 })
  const edit = page.getByRole('button', { name: 'mobil.example.org A bearbeiten' })
  await expect(edit).toBeVisible()
  expect((await edit.boundingBox())!.x + 36).toBeLessThanOrEqual(390)
  await expectNoHorizontalOverflow(page)
})
