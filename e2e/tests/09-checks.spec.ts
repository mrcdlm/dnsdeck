import http from 'node:http'
import type { AddressInfo } from 'node:net'

import { expect, test, type Page } from '@playwright/test'

import { api, expectNoHorizontalOverflow, login } from './helpers'

/** Lokaler Dienst, dessen Antwortcode sich umschalten lässt. */
async function startService() {
  let status = 200
  const server = http.createServer((_req, res) => {
    res.statusCode = status
    res.end('ok')
  })
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r))
  const port = (server.address() as AddressInfo).port
  return {
    url: `http://127.0.0.1:${port}/`,
    setStatus: (s: number) => (status = s),
    close: () => server.close(),
  }
}

const row = (page: Page, url: string) => page.locator('tr').filter({ has: page.getByRole('link', { name: url }) })

test.beforeEach(async ({ page }) => {
  await login(page)
})

test('Erreichbarkeit: anlegen, Entprellung, pausieren, entfernen', async ({ page }) => {
  const svc = await startService()
  try {
    await page.getByRole('link', { name: 'Erreichbarkeit' }).first().click()
    await expect(page).toHaveURL('/erreichbarkeit')

    await page.getByLabel('Adresse').fill(svc.url)
    await page.getByRole('button', { name: 'Prüfung hinzufügen' }).click()
    const r = row(page, svc.url)
    await expect(r.getByText('Erreichbar', { exact: true })).toBeVisible()
    await expect(r.getByText('200')).toBeVisible()
    await expect(r.getByText('kein TLS')).toBeVisible()

    // Doppelt
    await page.getByLabel('Adresse').fill(svc.url)
    await page.getByRole('button', { name: 'Prüfung hinzufügen' }).click()
    await expect(page.getByRole('alert')).toContainText('wird bereits geprüft')

    // Erster Fehlschlag ändert den Status noch nicht, der zweite schon
    svc.setStatus(503)
    await r.getByRole('button', { name: `${svc.url} jetzt prüfen` }).click()
    await expect(r.getByText(/wird erneut geprüft: Serverfehler HTTP 503/)).toBeVisible()
    await expect(r.getByText('Erreichbar', { exact: true })).toBeVisible()
    await r.getByRole('button', { name: `${svc.url} jetzt prüfen` }).click()
    await expect(r.getByText('Nicht erreichbar', { exact: true })).toBeVisible()

    // Dashboard-Kachel
    await page.getByRole('link', { name: 'Dashboard' }).first().click()
    await expect(page.getByText('1 gestört')).toBeVisible()
    await page.getByRole('link', { name: 'Erreichbarkeit' }).first().click()

    // Pausieren
    await r.getByRole('switch', { name: `${svc.url} prüfen` }).click()
    await expect(r.getByText('Pausiert')).toBeVisible()
    await expect(r.getByRole('button', { name: `${svc.url} jetzt prüfen` })).toBeDisabled()

    // Entfernen
    await r.getByRole('button', { name: `${svc.url} entfernen` }).click()
    await page.getByRole('alertdialog').getByRole('button', { name: 'Entfernen' }).click()
    await expect(r).toHaveCount(0)
  } finally {
    svc.close()
  }
})

test('Erreichbarkeit eines Records per Schalter im Dialog', async ({ page }) => {
  await page.goto('/records')
  await page.getByRole('button', { name: 'Hinzufügen' }).first().click()
  const dlg = page.getByRole('dialog')
  await dlg.getByRole('combobox', { name: 'Zone' }).click()
  await page.getByRole('option', { name: 'example.com', exact: true }).click()
  await dlg.getByLabel('Name').fill('dienst')
  await expect(dlg.getByText('Ruft https://dienst.example.com/ regelmäßig auf')).toBeVisible()
  await dlg.getByRole('switch', { name: 'Erreichbarkeit prüfen' }).click()
  await dlg.getByRole('button', { name: 'Hinzufügen' }).click()
  await expect(dlg).toBeHidden()

  // Symbol am Record, verlinkt auf die Übersicht
  await page.getByRole('link', { name: /^Erreichbarkeit: / }).first().click()
  await expect(page).toHaveURL('/erreichbarkeit')
  const r = row(page, 'https://dienst.example.com/')
  await expect(r.getByText('aus Record')).toBeVisible()

  // Beim Bearbeiten ist der Schalter an; ausschalten entfernt die Prüfung
  await page.goto('/records')
  await page.getByRole('button', { name: 'dienst.example.com A bearbeiten' }).first().click()
  const edit = page.getByRole('dialog')
  await expect(edit.getByRole('switch', { name: 'Erreichbarkeit prüfen' })).toBeChecked()
  await edit.getByRole('switch', { name: 'Erreichbarkeit prüfen' }).click()
  await edit.getByRole('button', { name: 'Speichern' }).click()
  await expect(edit).toBeHidden()
  const probes = (await api<{ url: string }[]>(page, 'GET', '/api/probes')).json
  expect(probes.some((p) => p.url === 'https://dienst.example.com/')).toBe(false)

  const recs = (await api<{ id: number; name: string }[]>(page, 'GET', '/api/records')).json
  for (const rec of recs.filter((x) => x.name === 'dienst.example.com')) {
    await api(page, 'DELETE', `/api/records/${rec.id}`)
  }
})

test('Erreichbarkeit: Einstellungen und mobile Ansicht', async ({ page }) => {
  await page.goto('/einstellungen')
  await page.getByRole('combobox', { name: 'Vor Zertifikatsablauf warnen' }).click()
  await page.getByRole('option', { name: '30 Tage' }).click()
  await page.getByRole('button', { name: 'Speichern' }).click()
  await expect(page.getByText('Gespeichert')).toBeVisible()
  expect((await api<{ tls_warn_days: number }>(page, 'GET', '/api/settings')).json.tls_warn_days).toBe(30)

  await page.setViewportSize({ width: 375, height: 800 })
  await page.goto('/erreichbarkeit')
  await expect(page.getByText(/30 Tage vor Ablauf gewarnt/)).toBeVisible()
  await expectNoHorizontalOverflow(page)
})

test('Erreichbarkeit: erwarteter Status, Bearbeiten, Verlauf', async ({ page }) => {
  const svc = await startService()
  try {
    await page.goto('/erreichbarkeit')
    await page.getByLabel('Adresse').fill(svc.url)
    await page.getByLabel('Erwarteter Status').fill('204')
    await page.getByRole('button', { name: 'Prüfung hinzufügen' }).click()
    const r = row(page, svc.url)
    // Dienst liefert 200, erwartet ist 204 → sofort gestört
    await expect(r.getByText('Unerwarteter Status HTTP 200 (erwartet: 204)')).toBeVisible()
    await expect(r.getByText('erwartet: 204', { exact: true })).toBeVisible()
    await expect(r.getByRole('img', { name: /Uptime 24 Stunden/ })).toBeVisible()

    // Bearbeiten: Erwartung auf 2xx → wieder erreichbar
    await r.getByRole('button', { name: `${svc.url} bearbeiten` }).click()
    const dlg = page.getByRole('dialog')
    await dlg.getByLabel('Erwarteter Status').fill('2xx')
    await dlg.getByRole('button', { name: 'Speichern' }).click()
    await expect(dlg).toBeHidden()
    await expect(r.getByText('Erreichbar', { exact: true })).toBeVisible()
    await expect(r.getByText('erwartet: 2xx', { exact: true })).toBeVisible()

    // Zeitraum umschalten
    await page.getByRole('button', { name: '7 Tage' }).click()
    await expect(r.getByRole('img', { name: /Uptime 7 Tage/ })).toBeVisible()

    await r.getByRole('button', { name: `${svc.url} entfernen` }).click()
    await page.getByRole('alertdialog').getByRole('button', { name: 'Entfernen' }).click()
    await expect(r).toHaveCount(0)
  } finally {
    svc.close()
  }
})
