import http from 'node:http'
import type { AddressInfo } from 'node:net'

import { expect, type Page } from '@playwright/test'

export const CF = 'http://localhost:8787'

export async function login(page: Page) {
  await page.goto('/login')
  await page.fill('input[name=password]', 'test')
  await page.click('button[type=submit]')
  await expect(page).toHaveURL('/')
}

/** API-Aufruf im Kontext der Seite (mit Session-Cookie). */
export async function api<T = unknown>(page: Page, method: string, path: string, body?: unknown) {
  return page.evaluate(
    async ([m, p, b]) => {
      const r = await fetch(p as string, {
        method: m as string,
        headers: { 'Content-Type': 'application/json' },
        body: b === undefined ? undefined : JSON.stringify(b),
      })
      return { status: r.status, json: r.status === 204 ? null : await r.json() }
    },
    [method, path, body] as const,
  ) as Promise<{ status: number; json: T }>
}

// Steuerung der nachgebauten Cloudflare-API
export const cf = {
  records: () => fetch(`${CF}/_records`).then((r) => r.json() as Promise<CFRecord[]>),
  fail: (status: number) => fetch(`${CF}/_fail?status=${status}`, { method: 'POST' }),
  tunnel: (name: string, status: string) => fetch(`${CF}/_tunnel?name=${name}&status=${status}`, { method: 'POST' }),
  /** Änderung wie im Cloudflare-Dashboard */
  async patch(zone: string, id: string, patch: Record<string, unknown>) {
    await fetch(`${CF}/client/v4/zones/${zone}/dns_records/${id}`, {
      method: 'PATCH',
      headers: { Authorization: 'Bearer dev', 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    })
  },
  async create(zone: string, rec: Record<string, unknown>) {
    const r = await fetch(`${CF}/client/v4/zones/${zone}/dns_records`, {
      method: 'POST',
      headers: { Authorization: 'Bearer dev', 'Content-Type': 'application/json' },
      body: JSON.stringify(rec),
    })
    return ((await r.json()) as { result: CFRecord }).result
  },
}

export interface CFRecord {
  id: string
  zone_id: string
  name: string
  type: string
  content: string
  ttl: number
  proxied: boolean
  comment?: string
}

/** Kein horizontales Scrollen der Seite (mobile Ansicht). */
export async function expectNoHorizontalOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
}

export interface Received {
  method: string
  path: string
  headers: http.IncomingHttpHeaders
  body: string
}

/** Lokaler Webhook-Empfänger. */
export async function startReceiver() {
  const received: Received[] = []
  const server = http.createServer((req, res) => {
    let body = ''
    req.on('data', (c) => (body += c))
    req.on('end', () => {
      received.push({ method: req.method ?? '', path: req.url ?? '', headers: req.headers, body })
      res.end('ok')
    })
  })
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r))
  const port = (server.address() as AddressInfo).port
  return { received, url: `http://127.0.0.1:${port}`, close: () => server.close() }
}
