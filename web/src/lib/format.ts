import { useEffect, useState } from 'react'

import i18n, { currentLang } from '@/i18n'

const locales = { de: 'de-DE', en: 'en-US' } as const

// Formatierer je Sprache zwischenspeichern
const cache = new Map<
  string,
  { rtf: Intl.RelativeTimeFormat; dtf: Intl.DateTimeFormat; nf: Intl.NumberFormat; regions: Intl.DisplayNames }
>()
function fmt() {
  const lang = currentLang()
  let f = cache.get(lang)
  if (!f) {
    f = {
      rtf: new Intl.RelativeTimeFormat(lang, { numeric: 'auto' }),
      dtf: new Intl.DateTimeFormat(locales[lang], { dateStyle: 'medium', timeStyle: 'short' }),
      nf: new Intl.NumberFormat(locales[lang], { maximumFractionDigits: 2 }),
      regions: new Intl.DisplayNames(locales[lang], { type: 'region' }),
    }
    cache.set(lang, f)
  }
  return f
}

const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['week', 7 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
]

export function relativeTime(iso: string, now: number): string {
  const diff = (new Date(iso).getTime() - now) / 1000
  const abs = Math.abs(diff)
  if (abs < 45) return i18n.t('time.justNow')
  for (const [unit, secs] of units) {
    if (abs >= secs) return fmt().rtf.format(Math.round(diff / secs), unit)
  }
  return fmt().rtf.format(Math.round(diff / 60), 'minute')
}

export function absoluteTime(iso: string): string {
  return fmt().dtf.format(new Date(iso))
}

export function formatNumber(n: number): string {
  return fmt().nf.format(n)
}

/** Liefert die aktuelle Zeit und aktualisiert sie regelmäßig (für relative Angaben). */
/** Ländername zum ISO-Code, z. B. "DE" → "Deutschland"; unbekannte Codes unverändert. */
export function countryName(code: string): string {
  try {
    return fmt().regions.of(code) ?? code
  } catch {
    return code
  }
}

export function useNow(intervalMs = 30_000): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs)
    return () => clearInterval(id)
  }, [intervalMs])
  return now
}
