import { expect, test } from '@playwright/test'

import { api, login } from './helpers'

// cfmock liefert auf :8553 den aktuellen Stand, auf :8554 den von vor 15 s.
test('Verbreitung: erst teilweise, dann überall; Details und „Jetzt prüfen“', async ({ page }) => {
  test.setTimeout(90_000)
  await login(page)
  const zone = (await api<{ id: string; name: string }[]>(page, 'GET', '/api/zones')).json[0]
  const created = Date.now()
  const rec = (
    await api<{ id: number }>(page, 'POST', '/api/records', {
      zone_id: zone.id, name: `verbreitung.${zone.name}`, type: 'A', proxied: false, ttl: 1, enabled: true,
    })
  ).json

  await page.goto('/records')
  const badge = page.getByRole('button', { name: `DNS-Verbreitung von verbreitung.${zone.name} A` })
  await expect(badge).toHaveText('wird geprüft …')
  // erste automatische Prüfung nach 10 s: der verzögerte Resolver kennt den Namen noch nicht
  await expect(badge).toHaveText('1/2', { timeout: 20_000 })

  await badge.click()
  const dlg = page.getByRole('dialog')
  await expect(dlg.getByText(/Teilweise verbreitet/)).toBeVisible()
  await expect(dlg.getByRole('row', { name: /Verzögert/ }).getByText('nicht vorhanden')).toBeVisible()
  await expect(dlg.getByText('wird automatisch wiederholt', { exact: false })).toBeVisible()

  // nach der Verzögerung liefert auch der zweite Resolver die IP
  await page.waitForTimeout(Math.max(0, created + 17_000 - Date.now()))
  await dlg.getByRole('button', { name: 'Jetzt prüfen' }).click()
  await expect(dlg.getByText('Überall aktuell')).toBeVisible()
  await expect(dlg.getByRole('row', { name: /Verzögert/ }).getByLabel('passt', { exact: true })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(badge).toHaveText('2/2')

  await api(page, 'DELETE', `/api/records/${rec.id}`)
})
