import type { TFunction } from 'i18next'

import type { SpeedtestResult } from './api'
import { countryName, formatNumber } from './format'

/** Mbit/s: ab 100 ganzzahlig, darunter eine Nachkommastelle. */
export function formatMbps(v: number): string {
  return formatNumber(v >= 100 ? Math.round(v) : Math.round(v * 10) / 10)
}

/** Millisekunden mit höchstens einer Nachkommastelle. */
export function formatMs(v: number): string {
  return formatNumber(v >= 10 ? Math.round(v) : Math.round(v * 10) / 10)
}

export function formatBytes(n: number): string {
  if (n >= 1e9) return `${formatNumber(Math.round(n / 1e8) / 10)} GB`
  return `${formatNumber(Math.round(n / 1e6))} MB`
}

export function succeeded(r: SpeedtestResult): boolean {
  return r.download_mbps !== undefined && !r.error
}

/** „Augsburg, Deutschland“ – soweit Cloudflare den Standort kennt. */
export function place(r: SpeedtestResult): string | undefined {
  const parts = [r.city, r.country && countryName(r.country)].filter(Boolean)
  return parts.length ? parts.join(', ') : undefined
}

export function formatInterval(t: TFunction, s: number): string {
  if (s % 86400 === 0) return t('interval.days', { count: s / 86400 })
  if (s % 3600 === 0) return t('interval.hours', { count: s / 3600 })
  return t('interval.minutes', { count: Math.round(s / 60) })
}
