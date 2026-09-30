import { expect, test } from '@playwright/test'

import { api, cf, expectNoHorizontalOverflow, login, startReceiver } from './helpers'

// Der Server läuft mit WEBHOOK_TEST_PATH=geheimer-pfad und WEBHOOK_CHAT=42.

let receiver: Awaited<ReturnType<typeof startReceiver>>

test.beforeAll(async () => {
  receiver = await startReceiver()
})
test.afterAll(() => receiver.close())

test.beforeEach(async ({ page }) => {
  await login(page)
  // Aufräumen: alle Webhooks löschen
  const list = (await api<{ id: number }[]>(page, 'GET', '/api/webhooks')).json
  for (const w of list) await api(page, 'DELETE', `/api/webhooks/${w.id}`)
  await page.goto('/einstellungen')
})

test.afterEach(async () => {
  await cf.fail(0)
  await cf.tunnel('home', 'healthy')
})

test('Webhook anlegen: Vorschau ohne Geheimnisse, Test, echte Zustellung, Deaktivieren', async ({ page }) => {
  await page.getByRole('button', { name: 'Webhook hinzufügen' }).click()
  const dlg = page.getByRole('dialog')
  await dlg.getByLabel('Name', { exact: true }).fill('Empfänger')
  await dlg.getByLabel('URL').fill(`${receiver.url}/\${WEBHOOK_TEST_PATH}`)
  await dlg.getByRole('button', { name: 'Header' }).click()
  await dlg.getByLabel('Header 1 Name').fill('X-Quelle')
  await dlg.getByLabel('Header 1 Wert').fill('dnsdeck-${WEBHOOK_CHAT}')
  await dlg.getByLabel('Body-Template').fill('{"chat": {{env "WEBHOOK_CHAT"}}, "text": {{json .Title}}, "typ": {{json .Type}}}')
  for (const label of ['IP-Wechsel', 'DNS-Update fehlgeschlagen', 'DNS-Update wieder OK']) {
    await dlg.getByRole('switch', { name: label }).click()
  }
  await dlg.getByRole('combobox', { name: 'Beispielereignis' }).click()
  await page.getByRole('option', { name: 'Beispiel: Tunnel-Statuswechsel' }).click()
  const pre = dlg.locator('pre')
  await expect(pre).toContainText('Tunnel home: getrennt')
  await expect(pre).toContainText('${WEBHOOK_TEST_PATH}')
  await expect(pre).toContainText('X-Quelle: dnsdeck-${WEBHOOK_CHAT}')
  await expect(pre).not.toContainText('geheimer-pfad')

  await dlg.getByRole('button', { name: 'Hinzufügen' }).click()
  await expect(dlg).toBeHidden()
  const card = page.locator('[data-webhook="Empfänger"]')
  await expect(card.getByText(`POST ${receiver.url}/\${WEBHOOK_TEST_PATH}`)).toBeVisible()
  expect(await page.content()).not.toContain('geheimer-pfad')

  // Test: Werte werden erst beim Senden eingesetzt (ungequotete Zahl ist gültig)
  await card.getByRole('button', { name: 'Testen' }).click()
  await expect(card.getByText('Testnachricht zugestellt')).toBeVisible()
  const t = receiver.received.at(-1)!
  expect(t.path).toBe('/geheimer-pfad')
  expect(JSON.parse(t.body).chat).toBe(42)
  expect(t.headers['x-quelle']).toBe('dnsdeck-42')

  // Echtes Ereignis
  const n = receiver.received.length
  await cf.tunnel('home', 'down')
  await api(page, 'POST', '/api/tunnels/refresh')
  await expect.poll(() => receiver.received.length).toBeGreaterThan(n)
  expect(JSON.parse(receiver.received.at(-1)!.body)).toMatchObject({ typ: 'tunnel_status', text: 'Tunnel home: getrennt' })

  // Deaktivieren → nichts mehr, Status bleibt erhalten
  await card.getByRole('switch', { name: 'Empfänger aktiv' }).click()
  await expect(card.getByText('inaktiv')).toBeVisible()
  const m = receiver.received.length
  await cf.tunnel('home', 'healthy')
  await api(page, 'POST', '/api/tunnels/refresh')
  await page.waitForTimeout(800)
  expect(receiver.received.length).toBe(m)
  // Aus-/Einschalten behält den Zustellstatus (anders als Änderungen an URL/Body)
  const saved = (await api<{ name: string; last_sent_at?: string; last_error?: string }[]>(page, 'GET', '/api/webhooks')).json
  expect(saved.find((w) => w.name === 'Empfänger')).toMatchObject({ last_sent_at: expect.any(String) })
  expect(saved.find((w) => w.name === 'Empfänger')?.last_error).toBeUndefined()
})

test('Vorlagen: Name wechselt mit, Gotify-Priorität, fehlende Variable', async ({ page }) => {
  await page.getByRole('button', { name: 'Webhook hinzufügen' }).click()
  const dlg = page.getByRole('dialog')
  await dlg.getByRole('combobox', { name: 'Vorlage' }).click()
  await page.getByRole('option', { name: 'ntfy' }).click()
  await expect(dlg.getByLabel('Name', { exact: true })).toHaveValue('ntfy')
  await dlg.getByRole('combobox', { name: 'Vorlage' }).click()
  await page.getByRole('option', { name: 'Gotify' }).click()
  await expect(dlg.getByLabel('Name', { exact: true })).toHaveValue('Gotify')
  await expect(dlg.getByLabel('Body-Template')).toHaveValue(/mul \.Priority 2/)
  await dlg.getByRole('combobox', { name: 'Beispielereignis' }).click()
  await page.getByRole('option', { name: 'Beispiel: Tunnel-Statuswechsel' }).click()
  await expect(dlg.locator('pre')).toContainText('"priority": 8')
  await expect(dlg.getByText('WEBHOOK_GOTIFY_TOKEN=…')).toBeVisible()

  await dlg.getByRole('button', { name: 'Hinzufügen' }).click()
  await expect(dlg).toBeHidden()
  const card = page.locator('[data-webhook="Gotify"]')
  await expect(card.getByText('WEBHOOK_GOTIFY_TOKEN', { exact: true })).toBeVisible()
  await card.getByRole('button', { name: 'Testen' }).click()
  await expect(card.getByText('Env-Variable WEBHOOK_GOTIFY_TOKEN ist nicht gesetzt')).toBeVisible()
})

test('Geheimnisse im Klartext und fremde Variablen werden abgelehnt', async ({ page }) => {
  await page.getByRole('button', { name: 'Webhook hinzufügen' }).click()
  const dlg = page.getByRole('dialog')
  await dlg.getByLabel('Name', { exact: true }).fill('Discord')
  await dlg.getByLabel('URL').fill('https://discord.com/api/webhooks/123/abcdef')
  await dlg.getByRole('button', { name: 'Hinzufügen' }).click()
  await expect(dlg.getByRole('alert').last()).toContainText('${WEBHOOK_NAME}')
  await dlg.getByRole('button', { name: 'Abbrechen' }).click()

  const bad = await api<{ error: string }>(page, 'POST', '/api/webhooks', {
    name: 'böse', enabled: true, method: 'POST', url: 'https://evil.example/${CF_API_TOKEN}',
    headers: [], content_type: 'application/json', body_template: '', events: [],
  })
  expect(bad.status).toBe(400)
  expect(bad.json.error).toContain('WEBHOOK_')
})

test('Bearbeiten, Löschen, mobile Ansicht', async ({ page }) => {
  expect((await api(page, 'POST', '/api/webhooks', {
    name: 'Mobil', enabled: true, method: 'POST', url: 'https://example.com/hook',
    headers: [], content_type: 'application/json', body_template: '', events: ['ip_change'],
  })).status).toBe(201)
  await page.reload()
  const card = page.locator('[data-webhook="Mobil"]')
  await card.getByRole('button', { name: 'Mobil bearbeiten' }).click()
  await expect(page.getByRole('dialog').getByLabel('URL')).toHaveValue('https://example.com/hook')
  await page.setViewportSize({ width: 390, height: 844 })
  await expectNoHorizontalOverflow(page)
  await page.getByRole('dialog').getByRole('button', { name: 'Abbrechen' }).click()
  await expectNoHorizontalOverflow(page)

  await card.getByRole('button', { name: 'Mobil löschen' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Löschen' }).click()
  await expect(card).toHaveCount(0)
})
